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

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
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

type imageBuildTarget struct {
	functionName string
	imageRef     string
	dockerfile   string
}

func prepareImages(req prepareImagesInput) error {
	manifestPath := strings.TrimSpace(req.ArtifactPath)
	if manifestPath == "" {
		return artifactcore.ErrArtifactPathRequired
	}
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		return err
	}
	runner := req.Runner
	useDefaultRunner := false
	if runner == nil {
		runner = defaultCommandRunner{}
		useDefaultRunner = true
	}
	ensureBase := req.EnsureBase || useDefaultRunner
	builtFunctionImages := make(map[string]struct{})
	builtBaseImages := make(map[string]struct{})

	for i := range manifest.Artifacts {
		artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, i)
		if err != nil {
			return err
		}
		runtimeConfigDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, i)
		if err != nil {
			return err
		}
		functionsPath := filepath.Join(runtimeConfigDir, "functions.yml")
		functionsPayload, ok, err := loadYAML(functionsPath)
		if err != nil {
			return fmt.Errorf("load functions config: %w", err)
		}
		if !ok {
			return fmt.Errorf("functions config not found: %s", functionsPath)
		}
		functionsRaw, ok := functionsPayload["functions"].(map[string]any)
		if !ok {
			return fmt.Errorf("functions must be map in %s", functionsPath)
		}

		buildTargets := collectImageBuildTargets(artifactRoot, functionsRaw, builtFunctionImages)
		if len(buildTargets) == 0 {
			continue
		}

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
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
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
		if _, ok := builtFunctionImages[imageRef]; ok {
			continue
		}
		dockerfile := filepath.Join(artifactRoot, "functions", functionName, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			continue
		}
		targets = append(targets, imageBuildTarget{
			functionName: functionName,
			imageRef:     imageRef,
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
		if err := ensureLambdaBaseImage(
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
	runtimeRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")), "/")
	hostRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")), "/")
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", nil, fmt.Errorf("read dockerfile %s: %w", dockerfile, err)
	}
	rewritten, changed := rewriteDockerfileForBuild(
		string(data),
		runtimeRegistry,
		hostRegistry,
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

func rewriteDockerfileForBuild(content, runtimeRegistry, hostRegistry, lambdaBaseTag string) (string, bool) {
	runtimePrefix := runtimeRegistry + "/"
	hostPrefix := hostRegistry + "/"
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
		rewrittenRef, rewritten := rewriteDockerfileFromRef(parts[refIndex], runtimePrefix, hostPrefix, lambdaBaseTag)
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

func rewriteDockerfileFromRef(ref, runtimePrefix, hostPrefix, lambdaBaseTag string) (string, bool) {
	current := strings.TrimSpace(ref)
	if current == "" {
		return ref, false
	}
	rewritten := current
	changed := false

	if runtimePrefix != "/" && hostPrefix != "/" && runtimePrefix != hostPrefix {
		if strings.HasPrefix(rewritten, runtimePrefix) {
			rewritten = hostPrefix + strings.TrimPrefix(rewritten, runtimePrefix)
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

func resolveLambdaBaseTag(runtime *artifactcore.RuntimeObservation) string {
	if runtime != nil {
		if tag := strings.TrimSpace(runtime.ESBVersion); tag != "" {
			return tag
		}
	}
	if tag := strings.TrimSpace(os.Getenv("ESB_TAG")); tag != "" {
		return tag
	}
	return ""
}

func ensureLambdaBaseImage(
	functionDockerfile string,
	noCache bool,
	runner CommandRunner,
	builtBaseImages map[string]struct{},
) error {
	baseImageRef, ok, err := readLambdaBaseRef(functionDockerfile)
	if err != nil {
		return err
	}
	if !ok || baseImageRef == "" {
		return nil
	}
	if _, done := builtBaseImages[baseImageRef]; done {
		return nil
	}
	if dockerImageExists(baseImageRef) {
		builtBaseImages[baseImageRef] = struct{}{}
		return nil
	}

	baseDockerfile, buildContext, err := resolveRuntimeHooksBuildPaths()
	if err != nil {
		return err
	}
	if err := runner.Run(buildxBuildCommand(baseImageRef, baseDockerfile, buildContext, noCache)); err != nil {
		return fmt.Errorf("build lambda base image %s: %w", baseImageRef, err)
	}
	pushRef := resolvePushReference(baseImageRef)
	if pushRef != baseImageRef {
		if err := runner.Run([]string{"docker", "tag", baseImageRef, pushRef}); err != nil {
			return fmt.Errorf("tag lambda base image %s -> %s: %w", baseImageRef, pushRef, err)
		}
	}
	if err := runner.Run([]string{"docker", "push", pushRef}); err != nil {
		return fmt.Errorf("push lambda base image %s: %w", pushRef, err)
	}
	builtBaseImages[baseImageRef] = struct{}{}
	builtBaseImages[pushRef] = struct{}{}
	return nil
}

func resolveRuntimeHooksBuildPaths() (dockerfilePath, buildContext string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("resolve working directory: %w", err)
	}
	current := start
	for {
		candidate := filepath.Join(current, "runtime-hooks", "python", "docker", "Dockerfile")
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", "", fmt.Errorf(
		"lambda base image %q not found locally and runtime hooks dockerfile is unavailable (expected: runtime-hooks/python/docker/Dockerfile from working tree root)",
		"esb-lambda-base",
	)
}

func readLambdaBaseRef(dockerfilePath string) (string, bool, error) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return "", false, fmt.Errorf("read dockerfile %s: %w", dockerfilePath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "from ") {
			continue
		}
		parts := strings.Fields(trimmed)
		refIndex := fromImageTokenIndex(parts)
		if refIndex < 0 || refIndex >= len(parts) {
			continue
		}
		ref := strings.TrimSpace(parts[refIndex])
		if isLambdaBaseRef(ref) {
			return ref, true, nil
		}
	}
	return "", false, nil
}

func isLambdaBaseRef(imageRef string) bool {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return false
	}
	withoutDigest := strings.SplitN(ref, "@", 2)[0]
	if withoutDigest == "" {
		return false
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
	return lastSegment == "esb-lambda-base"
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
	cmd = append(cmd, "--tag", tag, "--file", dockerfile, contextDir)
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
