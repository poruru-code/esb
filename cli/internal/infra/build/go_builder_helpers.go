// Where: cli/internal/infra/build/go_builder_helpers.go
// What: Helper utilities for GoBuilder image builds and staging.
// Why: Keep GoBuilder focused on orchestration logic.
package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"github.com/poruru/edge-serverless-box/meta"
)

type registryConfig struct {
	Registry string
}

func resolveRegistryConfig() (registryConfig, error) {
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		return registryConfig{}, err
	}
	registry := strings.TrimSpace(os.Getenv(key))
	if registry == "" {
		registry = constants.DefaultContainerRegistry
	}
	if !strings.HasSuffix(registry, "/") {
		registry += "/"
	}
	return registryConfig{Registry: registry}, nil
}

func defaultGeneratorParameters() map[string]string {
	return map[string]string{
		"S3_ENDPOINT_HOST":       "s3-storage",
		"DYNAMODB_ENDPOINT_HOST": "database",
	}
}

func toAnyMap(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func brandingImageLabels(project, env string) map[string]string {
	labels := map[string]string{
		compose.ESBManagedLabel: "true",
	}
	if trimmed := strings.TrimSpace(project); trimmed != "" {
		labels[compose.ESBProjectLabel] = trimmed
	}
	if trimmed := strings.TrimSpace(env); trimmed != "" {
		labels[compose.ESBEnvLabel] = trimmed
	}
	return labels
}

func stageConfigFiles(outputDir, repoRoot, composeProject, env string) error {
	configDir := filepath.Join(outputDir, "config")
	stagingRoot := staging.BaseDir(composeProject, env)

	// Verify source config files exist
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml"} {
		src := filepath.Join(configDir, name)
		if !fileExists(src) {
			return fmt.Errorf("config not found: %s", src)
		}
	}

	// Merge config files into CONFIG_DIR (with locking and atomic updates)
	if err := MergeConfig(outputDir, composeProject, env); err != nil {
		return err
	}

	// Stage pyproject.toml for isolated builds
	rootPyProject := filepath.Join(repoRoot, "pyproject.toml")
	if fileExists(rootPyProject) {
		destPyProject := filepath.Join(stagingRoot, "pyproject.toml")
		if err := copyFile(rootPyProject, destPyProject); err != nil {
			return err
		}
	}

	// Stage services/common and services/gateway for standardized structure
	commonSrc := filepath.Join(repoRoot, "services", "common")
	commonDest := filepath.Join(stagingRoot, "services", "common")
	gatewaySrc := filepath.Join(repoRoot, "services", "gateway")
	gatewayDest := filepath.Join(stagingRoot, "services", "gateway")

	// Clean staging services dir
	if err := removeDir(filepath.Join(stagingRoot, "services")); err != nil {
		return err
	}

	// Copy common
	if err := copyDir(commonSrc, commonDest); err != nil {
		return err
	}

	// Copy gateway source (excluding staging dir to avoid infinite recursion)
	entries, err := os.ReadDir(gatewaySrc)
	if err != nil {
		return err
	}
	if err := ensureDir(gatewayDest); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == meta.StagingDir {
			continue
		}
		srcPath := filepath.Join(gatewaySrc, entry.Name())
		dstPath := filepath.Join(gatewayDest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func withBuildLock(name string, fn func() error) error {
	key := strings.TrimSpace(name)
	if key == "" {
		return fn()
	}
	lockRoot := staging.RootDir()
	if err := os.MkdirAll(lockRoot, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockRoot, fmt.Sprintf(".lock-%s", key))
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

func lambdaBaseImageTag(registry, tag string) string {
	imageTag := fmt.Sprintf("%s-lambda-base:%s", meta.ImagePrefix, tag)
	return joinRegistry(registry, imageTag)
}

func joinRegistry(registry, image string) string {
	if registry == "" {
		return image
	}
	if strings.HasSuffix(registry, "/") {
		return registry + image
	}
	return registry + "/" + image
}

func buildFunctionImages(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	outputDir string,
	functions []template.FunctionSpec,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
	cacheRoot string,
	includeDocker bool,
) error {
	if verbose {
		fmt.Println("Building function images...")
	}
	proxyArgs := dockerBuildArgMap()
	expectedFingerprint := strings.TrimSpace(labels[compose.ESBImageFingerprintLabel])
	bakeTargets := make([]bakeTarget, 0, len(functions))
	builtFunctions := make([]string, 0, len(functions))
	for _, fn := range functions {
		if verbose {
			fmt.Printf("  Building image for %s...\n", fn.Name)
		}
		if strings.TrimSpace(fn.Name) == "" {
			return fmt.Errorf("function name is required")
		}
		if strings.TrimSpace(fn.ImageName) == "" {
			return fmt.Errorf("function image name is required for %s", fn.Name)
		}
		functionDir := filepath.Join(outputDir, "functions", fn.Name)
		dockerfile := filepath.Join(functionDir, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			return fmt.Errorf("dockerfile not found: %w", err)
		}
		if err := writeFunctionDockerignore(outputDir, functionDir); err != nil {
			return err
		}

		imageTag := fmt.Sprintf("%s-%s:%s", meta.ImagePrefix, fn.ImageName, tag)
		imageTag = joinRegistry(registry, imageTag)

		skipBuild := false
		if !noCache && expectedFingerprint != "" {
			if dockerImageHasLabelValue(ctx, runner, outputDir, imageTag, compose.ESBImageFingerprintLabel, expectedFingerprint) {
				skipBuild = true
				if verbose {
					fmt.Printf("  Skipping %s (up-to-date)\n", fn.Name)
				} else {
					fmt.Printf("  - Skipped function image (up-to-date): %s\n", fn.Name)
				}
			}
		}
		if !skipBuild {
			target := bakeTarget{
				Name:       "fn-" + fn.ImageName,
				Context:    outputDir,
				Dockerfile: dockerfile,
				Tags:       []string{imageTag},
				Outputs:    resolveBakeOutputs(registry, true, includeDocker),
				Labels:     labels,
				Args:       proxyArgs,
				NoCache:    noCache,
			}
			if err := applyBakeLocalCache(&target, cacheRoot, "functions"); err != nil {
				return err
			}
			bakeTargets = append(bakeTargets, target)
			builtFunctions = append(builtFunctions, fn.Name)
		}
	}

	if len(bakeTargets) > 0 {
		if err := runBakeGroup(
			ctx,
			runner,
			repoRoot,
			"esb-functions",
			bakeTargets,
			verbose,
		); err != nil {
			return err
		}
		if !verbose {
			for _, name := range builtFunctions {
				fmt.Printf("  - Built function image: %s\n", name)
			}
		}
	}
	return nil
}

func resolveRootCAPath() (string, error) {
	value, err := envutil.GetHostEnv(constants.HostSuffixCACertPath)
	if err != nil {
		return "", err
	}
	if value := strings.TrimSpace(value); value != "" {
		return ensureRootCAPath(expandHome(value))
	}
	value, err = envutil.GetHostEnv(constants.HostSuffixCertDir)
	if err != nil {
		return "", err
	}
	if value := strings.TrimSpace(value); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), meta.RootCACertFilename))
	}
	if value := strings.TrimSpace(os.Getenv("CAROOT")); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), meta.RootCACertFilename))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("root CA not found: %w", err)
	}
	return ensureRootCAPath(filepath.Join(home, meta.HomeDir, "certs", meta.RootCACertFilename))
}

func ensureRootCAPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("root CA path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("root CA not found at %s (run mise run setup:certs)", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("root CA path is a directory: %s", path)
	}
	return path, nil
}

func resolveRootCAFingerprint() (string, error) {
	path, err := resolveRootCAPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read root CA: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:4]), nil
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func writeFunctionDockerignore(contextDir, functionDir string) error {
	rel, err := filepath.Rel(contextDir, functionDir)
	if err != nil {
		return fmt.Errorf("resolve dockerignore path: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return nil
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || parts[0] != "functions" {
		return nil
	}

	lines := []string{
		fmt.Sprintf("# Auto-generated by %s build.", meta.AppName),
		"# What: Limit Docker build context to the active function and its layers.",
		"# Why: Reduce context upload size when using output_dir as build context.",
		"*",
		"!.dockerignore",
		"!functions/",
		"!" + rel + "/",
		"!" + rel + "/**",
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(contextDir, ".dockerignore"), []byte(content), 0o600)
}
