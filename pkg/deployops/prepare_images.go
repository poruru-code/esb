package deployops

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
	"gopkg.in/yaml.v3"
)

type prepareImagesInput struct {
	ArtifactPath string
	NoCache      bool
	Runner       CommandRunner
	Runtime      *artifactcore.RuntimeObservation
	EnsureBase   bool
}

type CommandRunner interface {
	Run(cmd []string) error
}

type defaultCommandRunner struct{}

func (defaultCommandRunner) Run(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command is empty")
	}
	command := exec.Command(cmd[0], cmd[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

var defaultCommandRunnerFactory = func() CommandRunner {
	return defaultCommandRunner{}
}

var dockerImageExistsFunc = dockerImageExists

type imageBuildTarget struct {
	functionName string
	imageRef     string
	dockerfile   string
}

type prepareImagesResult struct {
	publishedFunctionImages map[string]struct{}
}

func prepareImages(req prepareImagesInput) error {
	_, err := prepareImagesWithResult(req)
	return err
}

func prepareImagesWithResult(req prepareImagesInput) (prepareImagesResult, error) {
	manifestPath := strings.TrimSpace(req.ArtifactPath)
	if manifestPath == "" {
		return prepareImagesResult{}, artifactcore.ErrArtifactPathRequired
	}
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		return prepareImagesResult{}, err
	}
	runner := req.Runner
	if runner == nil {
		runner = defaultCommandRunnerFactory()
	}
	ensureBase := req.EnsureBase
	builtFunctionImages := make(map[string]struct{})
	builtBaseImages := make(map[string]struct{})
	publishedFunctionImages := make(map[string]struct{})
	hasFunctionBuildTargets := false

	for i := range manifest.Artifacts {
		artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, i)
		if err != nil {
			return prepareImagesResult{}, err
		}
		runtimeConfigDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, i)
		if err != nil {
			return prepareImagesResult{}, err
		}
		functionsPath := filepath.Join(runtimeConfigDir, "functions.yml")
		functionsPayload, ok, err := loadYAML(functionsPath)
		if err != nil {
			return prepareImagesResult{}, fmt.Errorf("load functions config: %w", err)
		}
		if !ok {
			return prepareImagesResult{}, fmt.Errorf("functions config not found: %s", functionsPath)
		}
		functionsRaw, ok := functionsPayload["functions"].(map[string]any)
		if !ok {
			return prepareImagesResult{}, fmt.Errorf("functions must be map in %s", functionsPath)
		}

		buildTargets := collectImageBuildTargets(artifactRoot, functionsRaw, builtFunctionImages)
		if len(buildTargets) == 0 {
			continue
		}
		hasFunctionBuildTargets = true

		functionNames := make([]string, 0, len(buildTargets))
		for _, target := range buildTargets {
			functionNames = append(functionNames, target.functionName)
		}

		if err := withFunctionBuildWorkspace(artifactRoot, functionNames, func(contextRoot string) error {
			for _, target := range buildTargets {
				if err := buildAndPushFunctionImage(
					target.imageRef,
					target.functionName,
					contextRoot,
					req.NoCache,
					req.Runtime,
					runner,
					ensureBase,
					builtBaseImages,
				); err != nil {
					return err
				}
				builtFunctionImages[target.imageRef] = struct{}{}
				publishedFunctionImages[target.imageRef] = struct{}{}
			}
			return nil
		}); err != nil {
			return prepareImagesResult{}, err
		}
	}
	if ensureBase && !hasFunctionBuildTargets {
		defaultBaseRef, err := resolveDefaultLambdaBaseRef(req.Runtime)
		if err != nil {
			return prepareImagesResult{}, err
		}
		if err := ensureLambdaBaseImage(defaultBaseRef, req.NoCache, runner, builtBaseImages); err != nil {
			return prepareImagesResult{}, err
		}
	}
	return prepareImagesResult{publishedFunctionImages: publishedFunctionImages}, nil
}

func collectImageBuildTargets(
	artifactRoot string,
	functionsRaw map[string]any,
	builtFunctionImages map[string]struct{},
) []imageBuildTarget {
	names := make([]string, 0, len(functionsRaw))
	for name := range functionsRaw {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := make([]imageBuildTarget, 0, len(names))
	for _, functionName := range names {
		payload, ok := functionsRaw[functionName].(map[string]any)
		if !ok {
			continue
		}
		rawImageRef, ok := payload["image"]
		if !ok {
			continue
		}
		imageRef := strings.TrimSpace(fmt.Sprintf("%v", rawImageRef))
		if imageRef == "" {
			continue
		}
		normalizedImageRef, _ := normalizeFunctionImageRefForRuntime(imageRef)
		if normalizedImageRef == "" {
			continue
		}
		if _, ok := builtFunctionImages[normalizedImageRef]; ok {
			continue
		}
		dockerfile := filepath.Join(artifactRoot, "functions", functionName, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			continue
		}
		targets = append(targets, imageBuildTarget{
			functionName: functionName,
			imageRef:     normalizedImageRef,
			dockerfile:   dockerfile,
		})
	}
	return targets
}

func buildAndPushFunctionImage(
	imageRef, functionName, contextRoot string,
	noCache bool,
	runtime *artifactcore.RuntimeObservation,
	runner CommandRunner,
	ensureBase bool,
	builtBaseImages map[string]struct{},
) error {
	dockerfile := filepath.Join(contextRoot, "functions", functionName, "Dockerfile")
	pushRef := resolvePushReference(imageRef)
	resolvedDockerfile, cleanup, err := resolveFunctionBuildDockerfile(dockerfile, runtime)
	if err != nil {
		return err
	}
	defer cleanup()
	if ensureBase {
		if err := ensureLambdaBaseImageFromDockerfile(
			resolvedDockerfile,
			noCache,
			runner,
			builtBaseImages,
		); err != nil {
			return err
		}
	}

	buildCmd := buildxBuildCommand(imageRef, resolvedDockerfile, contextRoot, noCache)
	if err := runner.Run(buildCmd); err != nil {
		return fmt.Errorf("build function image %s: %w", imageRef, err)
	}
	if pushRef != imageRef {
		if err := runner.Run([]string{"docker", "tag", imageRef, pushRef}); err != nil {
			return fmt.Errorf("tag function image %s -> %s: %w", imageRef, pushRef, err)
		}
	}
	if err := runner.Run([]string{"docker", "push", pushRef}); err != nil {
		return fmt.Errorf("push function image %s: %w", pushRef, err)
	}
	return nil
}

func resolveFunctionBuildDockerfile(dockerfile string, runtime *artifactcore.RuntimeObservation) (string, func(), error) {
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", nil, fmt.Errorf("read dockerfile %s: %w", dockerfile, err)
	}
	rewritten, changed := rewriteDockerfileForBuild(
		string(data),
		resolveHostFunctionRegistry(),
		resolveRegistryAliases(),
		resolveLambdaBaseTag(runtime),
	)
	if !changed {
		return dockerfile, func() {}, nil
	}

	tmpPath := dockerfile + ".artifact.build"
	if err := os.WriteFile(tmpPath, []byte(rewritten), 0o644); err != nil {
		return "", nil, fmt.Errorf("write temporary dockerfile %s: %w", tmpPath, err)
	}
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	return tmpPath, cleanup, nil
}

func rewriteDockerfileForBuild(content, hostRegistry string, registryAliases []string, lambdaBaseTag string) (string, bool) {
	lines := strings.Split(content, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "from ") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		refIndex := fromImageTokenIndex(parts)
		if refIndex < 0 {
			continue
		}
		rewrittenRef, rewritten := rewriteDockerfileFromRef(parts[refIndex], hostRegistry, registryAliases, lambdaBaseTag)
		if !rewritten {
			continue
		}
		parts[refIndex] = rewrittenRef
		indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
		indent := line[:indentLen]
		lines[i] = indent + strings.Join(parts, " ")
		changed = true
	}
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

func rewriteDockerfileFromRef(ref, hostRegistry string, registryAliases []string, lambdaBaseTag string) (string, bool) {
	current := strings.TrimSpace(ref)
	if current == "" {
		return ref, false
	}
	rewritten := current
	changed := false

	if baseRef, refChanged := rewriteLambdaBaseRefForBuild(rewritten); refChanged {
		rewritten = baseRef
		changed = true
	} else if hostRegistry != "" {
		if baseRef, refChanged := rewriteRegistryAlias(rewritten, hostRegistry, registryAliases); refChanged {
			rewritten = baseRef
			changed = true
		}
	}

	if lambdaBaseTag == "" {
		return rewritten, changed
	}
	next, tagChanged := rewriteLambdaBaseTag(rewritten, lambdaBaseTag)
	if tagChanged {
		rewritten = next
		changed = true
	}
	return rewritten, changed
}

func rewriteLambdaBaseTag(imageRef, tag string) (string, bool) {
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag == "" {
		return imageRef, false
	}
	withoutDigest := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if withoutDigest == "" {
		return imageRef, false
	}

	slash := strings.LastIndex(withoutDigest, "/")
	colon := strings.LastIndex(withoutDigest, ":")
	repo := withoutDigest
	if colon > slash {
		repo = withoutDigest[:colon]
	}
	lastSegment := repo
	if slash >= 0 && slash+1 < len(repo) {
		lastSegment = repo[slash+1:]
	}
	if lastSegment != "esb-lambda-base" {
		return imageRef, false
	}

	// Keep explicit (non-latest) tags authored in artifact Dockerfiles.
	// Runtime tag override is only applied to floating latest references.
	if colon > slash {
		currentTag := withoutDigest[colon+1:]
		if currentTag != "" && currentTag != "latest" {
			return imageRef, false
		}
	}
	return repo + ":" + trimmedTag, true
}

func dockerImageExists(imageRef string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageRef)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func fromImageTokenIndex(parts []string) int {
	for i := 1; i < len(parts); i++ {
		token := strings.TrimSpace(parts[i])
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "--") {
			continue
		}
		return i
	}
	return -1
}

func buildxBuildCommand(tag, dockerfile, contextDir string, noCache bool) []string {
	cmd := []string{
		"docker",
		"buildx",
		"build",
		"--platform",
		"linux/amd64",
		"--load",
		"--pull",
	}
	if noCache {
		cmd = append(cmd, "--no-cache")
	}
	cmd = appendProxyBuildArgs(cmd)
	cmd = append(cmd, "--tag", tag, "--file", dockerfile, contextDir)
	return cmd
}

func appendProxyBuildArgs(cmd []string) []string {
	type proxyEnvPair struct {
		upper string
		lower string
	}
	pairs := []proxyEnvPair{
		{upper: "HTTP_PROXY", lower: "http_proxy"},
		{upper: "HTTPS_PROXY", lower: "https_proxy"},
		{upper: "NO_PROXY", lower: "no_proxy"},
	}

	for _, pair := range pairs {
		value := strings.TrimSpace(os.Getenv(pair.upper))
		if value == "" {
			value = strings.TrimSpace(os.Getenv(pair.lower))
		}
		if value == "" {
			continue
		}
		cmd = append(cmd, "--build-arg", pair.upper+"="+value)
		cmd = append(cmd, "--build-arg", pair.lower+"="+value)
	}
	return cmd
}

func resolvePushReference(imageRef string) string {
	runtimeRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")), "/")
	hostRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")), "/")
	if runtimeRegistry == "" || hostRegistry == "" {
		return imageRef
	}
	prefix := runtimeRegistry + "/"
	if !strings.HasPrefix(imageRef, prefix) {
		return imageRef
	}
	suffix := strings.TrimPrefix(imageRef, prefix)
	return hostRegistry + "/" + suffix
}

func withFunctionBuildWorkspace(
	artifactRoot string,
	functionNames []string,
	fn func(contextRoot string) error,
) error {
	normalized := sortedUniqueNonEmpty(functionNames)
	contextRoot, err := os.MkdirTemp("", "artifactctl-build-context-*")
	if err != nil {
		return fmt.Errorf("create temporary build context: %w", err)
	}
	defer os.RemoveAll(contextRoot)

	functionsRoot := filepath.Join(contextRoot, "functions")
	if err := os.MkdirAll(functionsRoot, 0o755); err != nil {
		return fmt.Errorf("create temporary functions context: %w", err)
	}
	for _, name := range normalized {
		sourceDir := filepath.Join(artifactRoot, "functions", name)
		targetDir := filepath.Join(functionsRoot, name)
		if err := copyDir(sourceDir, targetDir); err != nil {
			return fmt.Errorf("prepare function context %s: %w", name, err)
		}
	}
	return fn(contextRoot)
}

func copyDir(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("source is not directory: %s", source)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		targetPath := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(current)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}
		return copyFile(current, targetPath, info.Mode().Perm())
	})
}

func copyFile(source, target string, perm os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Close()
}

func loadYAML(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	result := map[string]any{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func sortedUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}
