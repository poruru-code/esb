package deployops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	proxymaven "github.com/poruru-code/esb/pkg/proxy/maven"
)

const mavenShimImagePrefix = "esb-maven-shim"

func ensureMavenShimImage(
	baseRef string,
	noCache bool,
	runner CommandRunner,
	resolvedMavenShimImages map[string]string,
) (string, error) {
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		return "", fmt.Errorf("maven base image reference is empty")
	}
	if shimRef, ok := resolvedMavenShimImages[baseRef]; ok {
		return shimRef, nil
	}

	hash := sha256.Sum256([]byte(baseRef))
	shortHash := hex.EncodeToString(hash[:])[:16]
	shimImage := fmt.Sprintf("%s:%s", mavenShimImagePrefix, shortHash)
	hostRegistry := strings.TrimSuffix(strings.TrimSpace(resolveHostFunctionRegistry()), "/")
	shimRef := shimImage
	if hostRegistry != "" {
		shimRef = hostRegistry + "/" + shimImage
	}

	if noCache || !dockerImageExistsFunc(shimRef) {
		if err := validateMavenShimProxyEnv(); err != nil {
			return "", err
		}
		dockerfilePath, contextRoot, err := resolveMavenShimBuildPaths()
		if err != nil {
			return "", err
		}
		buildCmd := buildxBuildCommandWithBuildArgs(
			shimRef,
			dockerfilePath,
			contextRoot,
			noCache,
			map[string]string{
				"BASE_MAVEN_IMAGE": baseRef,
			},
		)
		if err := runner.Run(buildCmd); err != nil {
			return "", fmt.Errorf("build maven shim image %s from %s: %w", shimRef, baseRef, err)
		}
	}
	if hostRegistry != "" {
		if err := runner.Run([]string{"docker", "push", shimRef}); err != nil {
			return "", fmt.Errorf("push maven shim image %s: %w", shimRef, err)
		}
	}

	resolvedMavenShimImages[baseRef] = shimRef
	return shimRef, nil
}

func validateMavenShimProxyEnv() error {
	env := map[string]string{
		"HTTP_PROXY":  os.Getenv("HTTP_PROXY"),
		"http_proxy":  os.Getenv("http_proxy"),
		"HTTPS_PROXY": os.Getenv("HTTPS_PROXY"),
		"https_proxy": os.Getenv("https_proxy"),
	}
	if _, err := proxymaven.ResolveEndpointsFromEnv(env); err != nil {
		return fmt.Errorf("invalid proxy configuration for maven shim build: %w", err)
	}
	return nil
}

func resolveMavenShimBuildPaths() (dockerfilePath, buildContext string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("resolve working directory: %w", err)
	}
	current := start
	for {
		candidate := filepath.Join(current, "tools", "maven-shim", "Dockerfile")
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			contextRoot := filepath.Join(current, "tools", "maven-shim")
			return candidate, contextRoot, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", "", fmt.Errorf(
		"maven shim Dockerfile is unavailable (expected: tools/maven-shim/Dockerfile from working tree root)",
	)
}
