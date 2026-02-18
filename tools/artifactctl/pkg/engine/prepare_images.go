package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type PrepareImagesRequest struct {
	ArtifactPath string
	NoCache      bool
	Runner       CommandRunner
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

const (
	runtimeBaseContextDirName      = "runtime-base"
	runtimeBasePythonDockerfileRel = "runtime-hooks/python/docker/Dockerfile"
)

func PrepareImages(req PrepareImagesRequest) error {
	manifestPath := strings.TrimSpace(req.ArtifactPath)
	if manifestPath == "" {
		return fmt.Errorf("artifact path is required")
	}
	manifest, err := ReadArtifactManifest(manifestPath)
	if err != nil {
		return err
	}
	runner := req.Runner
	if runner == nil {
		runner = defaultCommandRunner{}
	}
	builtBaseRuntimeRefs := make(map[string]struct{})
	builtFunctionImages := make(map[string]struct{})

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
		if err := withTemporaryFunctionContextDockerignore(artifactRoot, functionNames, func() error {
			for _, target := range buildTargets {
				baseRefs, err := collectDockerfileBaseImages(target.dockerfile)
				if err != nil {
					return err
				}
				for _, baseRef := range baseRefs {
					if !strings.Contains(baseRef, "esb-lambda-base:") {
						continue
					}
					if _, ok := builtBaseRuntimeRefs[baseRef]; ok {
						continue
					}
					if err := buildAndPushLambdaBaseImage(baseRef, artifactRoot, req.NoCache, runner); err != nil {
						return err
					}
					builtBaseRuntimeRefs[baseRef] = struct{}{}
				}
				if err := buildAndPushFunctionImage(target.imageRef, target.dockerfile, artifactRoot, req.NoCache, runner); err != nil {
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

func collectDockerfileBaseImages(dockerfile string) ([]string, error) {
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return nil, fmt.Errorf("read dockerfile %s: %w", dockerfile, err)
	}
	lines := strings.Split(string(data), "\n")
	refs := make([]string, 0)
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(stripped), "from ") {
			continue
		}
		parts := strings.Fields(stripped)
		if len(parts) < 2 {
			continue
		}
		ref := extractFromImageRef(parts)
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func extractFromImageRef(parts []string) string {
	for _, token := range parts[1:] {
		value := strings.TrimSpace(token)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "--") {
			continue
		}
		return value
	}
	return ""
}

func buildAndPushLambdaBaseImage(runtimeRef, artifactRoot string, noCache bool, runner CommandRunner) error {
	dockerfile, contextDir, err := resolveRuntimeBaseBuildContext(artifactRoot)
	if err != nil {
		return err
	}
	pushRef := resolvePushReference(runtimeRef)
	buildCmd := buildxBuildCommand(pushRef, dockerfile, contextDir, noCache)
	if err := runner.Run(buildCmd); err != nil {
		return fmt.Errorf("build lambda base image %s: %w", pushRef, err)
	}
	if runtimeRef != pushRef {
		if err := runner.Run([]string{"docker", "tag", pushRef, runtimeRef}); err != nil {
			return fmt.Errorf("tag lambda base image %s -> %s: %w", pushRef, runtimeRef, err)
		}
	}
	if err := runner.Run([]string{"docker", "push", pushRef}); err != nil {
		return fmt.Errorf("push lambda base image %s: %w", pushRef, err)
	}
	return nil
}

func resolveRuntimeBaseBuildContext(artifactRoot string) (string, string, error) {
	contextDir := filepath.Join(artifactRoot, runtimeBaseContextDirName)
	dockerfile := filepath.Join(contextDir, runtimeBasePythonDockerfileRel)
	if _, err := os.Stat(dockerfile); err != nil {
		return "", "", fmt.Errorf(
			"runtime base dockerfile not found: %s (run artifact generate to stage runtime-base)",
			dockerfile,
		)
	}
	return dockerfile, contextDir, nil
}

func buildAndPushFunctionImage(imageRef, dockerfile, artifactRoot string, noCache bool, runner CommandRunner) error {
	pushRef := resolvePushReference(imageRef)
	resolvedDockerfile, cleanup, err := resolveFunctionBuildDockerfile(dockerfile)
	if err != nil {
		return err
	}
	defer cleanup()

	buildCmd := buildxBuildCommand(imageRef, resolvedDockerfile, artifactRoot, noCache)
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

func resolveFunctionBuildDockerfile(dockerfile string) (string, func(), error) {
	runtimeRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")), "/")
	hostRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")), "/")
	if runtimeRegistry == "" || hostRegistry == "" || runtimeRegistry == hostRegistry {
		return dockerfile, func() {}, nil
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", nil, fmt.Errorf("read dockerfile %s: %w", dockerfile, err)
	}
	rewritten, changed := rewriteDockerfileFromRegistry(string(data), runtimeRegistry, hostRegistry)
	if !changed {
		return dockerfile, func() {}, nil
	}

	tmpPath := dockerfile + ".artifactctl.build"
	if err := os.WriteFile(tmpPath, []byte(rewritten), 0o644); err != nil {
		return "", nil, fmt.Errorf("write temporary dockerfile %s: %w", tmpPath, err)
	}
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	return tmpPath, cleanup, nil
}

func rewriteDockerfileFromRegistry(content, runtimeRegistry, hostRegistry string) (string, bool) {
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
		ref := parts[refIndex]
		if !strings.HasPrefix(ref, runtimePrefix) {
			continue
		}
		parts[refIndex] = hostPrefix + strings.TrimPrefix(ref, runtimePrefix)
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

func withTemporaryFunctionContextDockerignore(
	artifactRoot string,
	functionNames []string,
	fn func() error,
) error {
	dockerignore := filepath.Join(artifactRoot, ".dockerignore")
	original, hadOriginal, err := readFileIfExists(dockerignore)
	if err != nil {
		return err
	}
	if err := writeFunctionContextDockerignore(dockerignore, functionNames); err != nil {
		return err
	}
	defer func() {
		if hadOriginal {
			_ = os.WriteFile(dockerignore, original, 0o644)
			return
		}
		_ = os.Remove(dockerignore)
	}()
	return fn()
}

func writeFunctionContextDockerignore(path string, functionNames []string) error {
	normalized := sortedUniqueNonEmpty(functionNames)
	lines := []string{
		"# Auto-generated by artifactctl prepare-images.",
		"# What: Permit function build context for all functions in this artifact root.",
		"*",
		"!.dockerignore",
		"!functions/",
	}
	for _, name := range normalized {
		lines = append(lines, "!functions/"+name+"/")
		lines = append(lines, "!functions/"+name+"/**")
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func readFileIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}
