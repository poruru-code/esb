package deployops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveDefaultLambdaBaseRef() (string, error) {
	registry := resolveEnsureBaseRegistry()
	if registry == "" {
		return "", fmt.Errorf(
			"lambda base registry is unresolved: set CONTAINER_REGISTRY or HOST_REGISTRY_ADDR (or REGISTRY)",
		)
	}
	return registry + "/esb-lambda-base:latest", nil
}

func resolveEnsureBaseRegistry() string {
	for _, key := range []string{"HOST_REGISTRY_ADDR", "CONTAINER_REGISTRY", "REGISTRY"} {
		value := strings.TrimSuffix(strings.TrimSpace(os.Getenv(key)), "/")
		if value != "" {
			return value
		}
	}
	return ""
}

func ensureLambdaBaseImageFromDockerfile(
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
	return ensureLambdaBaseImage(baseImageRef, noCache, runner, builtBaseImages)
}

func ensureLambdaBaseImage(
	baseImageRef string,
	noCache bool,
	runner CommandRunner,
	builtBaseImages map[string]struct{},
) error {
	baseImageRef = strings.TrimSpace(baseImageRef)
	if baseImageRef == "" {
		return nil
	}
	pushRef := resolvePushReference(baseImageRef)
	if _, done := builtBaseImages[baseImageRef]; done {
		return nil
	}
	if _, done := builtBaseImages[pushRef]; done {
		builtBaseImages[baseImageRef] = struct{}{}
		return nil
	}
	if !dockerImageExistsFunc(baseImageRef) {
		pullErr := runner.Run([]string{"docker", "pull", baseImageRef})
		if pullErr != nil {
			baseDockerfile, buildContext, err := resolveRuntimeHooksBuildPaths()
			if err != nil {
				return fmt.Errorf("pull lambda base image %s: %v; %w", baseImageRef, pullErr, err)
			}
			if err := runner.Run(buildxBuildCommand(baseImageRef, baseDockerfile, buildContext, noCache)); err != nil {
				return fmt.Errorf("build lambda base image %s: %w", baseImageRef, err)
			}
		}
	}

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
