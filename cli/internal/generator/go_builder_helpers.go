// Where: cli/internal/generator/go_builder_helpers.go
// What: Helper utilities for GoBuilder image builds and staging.
// Why: Keep GoBuilder focused on orchestration logic.
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

type registryConfig struct {
	External string
	Internal string
}

const (
	esbRootCASecretID = "esb_root_ca"
	esbRootCACertName = "rootCA.crt"
)

func resolveRegistryConfig(mode string) registryConfig {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case compose.ModeContainerd, compose.ModeFirecracker:
		port := strings.TrimSpace(os.Getenv("ESB_PORT_REGISTRY"))
		if port == "" {
			port = "5010"
		}
		return registryConfig{
			External: fmt.Sprintf("localhost:%s", port),
			Internal: "registry:5010",
		}
	default:
		return registryConfig{}
	}
}

func resolveImageTag(env string) string {
	if tag := strings.TrimSpace(os.Getenv("ESB_IMAGE_TAG")); tag != "" {
		return tag
	}
	if strings.TrimSpace(env) != "" {
		return env
	}
	return "latest"
}

func defaultGeneratorParameters() map[string]string {
	return map[string]string{
		"S3_ENDPOINT_HOST":       "s3-storage",
		"DYNAMODB_ENDPOINT_HOST": "database",
	}
}

func esbImageLabels(project, env string) map[string]string {
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
	stagingRoot := staging.BaseDir(repoRoot, composeProject, env)
	destDir := filepath.Join(stagingRoot, env, "config")
	if err := removeDir(destDir); err != nil {
		return err
	}
	if err := ensureDir(destDir); err != nil {
		return err
	}

	for _, name := range []string{"functions.yml", "routing.yml"} {
		src := filepath.Join(configDir, name)
		if !fileExists(src) {
			return fmt.Errorf("config not found: %s", src)
		}
		if err := copyFile(src, filepath.Join(destDir, name)); err != nil {
			return err
		}
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

	// Copy gateway source (excluding .esb-staging to avoid infinite recursion)
	entries, err := os.ReadDir(gatewaySrc)
	if err != nil {
		return err
	}
	if err := ensureDir(gatewayDest); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".esb-staging" {
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

func ensureRegistryRunning(
	ctx context.Context,
	runner compose.CommandRunner,
	rootDir string,
	project string,
	mode string,
) error {
	if runner == nil {
		return fmt.Errorf("compose runner is nil")
	}
	files, err := compose.ResolveComposeFiles(rootDir, mode, "control")
	if err != nil {
		return err
	}
	args := []string{"compose"}
	if project != "" {
		args = append(args, "-p", project)
	}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	args = append(args, "up", "-d", "registry")
	return runner.Run(ctx, rootDir, "docker", args...)
}

func buildBaseImage(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
) error {
	if verbose {
		fmt.Println("Building base image...")
	}
	assetsDir := filepath.Join(repoRoot, "cli", "internal", "generator", "assets")
	dockerfile := filepath.Join(assetsDir, "Dockerfile.lambda-base")
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("base dockerfile not found: %w", err)
	}

	imageTag := fmt.Sprintf("esb-lambda-base:%s", tag)
	if registry != "" {
		imageTag = fmt.Sprintf("%s/%s", registry, imageTag)
	}

	if err := buildDockerImage(ctx, runner, assetsDir, "Dockerfile.lambda-base", imageTag, noCache, verbose, labels); err != nil {
		return err
	}
	if registry != "" {
		return pushDockerImage(ctx, runner, assetsDir, imageTag, verbose)
	}
	return nil
}

func buildFunctionImages(
	ctx context.Context,
	runner compose.CommandRunner,
	outputDir string,
	functions []FunctionSpec,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
) error {
	if verbose {
		fmt.Println("Building function images...")
	}
	for _, fn := range functions {
		if verbose {
			fmt.Printf("  Building image for %s...\n", fn.Name)
		}
		if strings.TrimSpace(fn.Name) == "" {
			return fmt.Errorf("function name is required")
		}
		functionDir := filepath.Join(outputDir, "functions", fn.Name)
		dockerfile := filepath.Join(functionDir, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			return fmt.Errorf("dockerfile not found: %w", err)
		}
		if err := writeFunctionDockerignore(outputDir, functionDir); err != nil {
			return err
		}

		imageTag := fmt.Sprintf("%s:%s", fn.Name, tag)
		if registry != "" {
			imageTag = fmt.Sprintf("%s/%s", registry, imageTag)
		}

		dockerfileRel, err := filepath.Rel(outputDir, dockerfile)
		if err != nil {
			return err
		}
		dockerfileRel = filepath.ToSlash(dockerfileRel)

		if err := buildDockerImage(ctx, runner, outputDir, dockerfileRel, imageTag, noCache, verbose, labels); err != nil {
			return err
		}
		if registry != "" {
			if err := pushDockerImage(ctx, runner, outputDir, imageTag, verbose); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildServiceImages(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
) error {
	if verbose {
		fmt.Println("Building service images...")
	}
	services := map[string]struct {
		dir        string
		dockerfile string
	}{
		"esb-runtime-node": {
			dir:        filepath.Join(repoRoot, "services", "runtime-node"),
			dockerfile: "Dockerfile.firecracker",
		},
		"esb-agent": {
			dir:        filepath.Join(repoRoot, "services", "agent"),
			dockerfile: "Dockerfile",
		},
	}
	for name, info := range services {
		dockerfile := filepath.Join(info.dir, info.dockerfile)
		if _, err := os.Stat(dockerfile); err != nil {
			return fmt.Errorf("service dockerfile not found: %w", err)
		}
		imageTag := fmt.Sprintf("%s:%s", name, tag)
		if registry != "" {
			imageTag = fmt.Sprintf("%s/%s", registry, imageTag)
		}
		if err := buildDockerImage(ctx, runner, info.dir, info.dockerfile, imageTag, noCache, verbose, labels); err != nil {
			return err
		}
		if registry != "" {
			if err := pushDockerImage(ctx, runner, info.dir, imageTag, verbose); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildDockerImage(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	dockerfile string,
	imageTag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if contextDir == "" {
		return fmt.Errorf("context dir is required")
	}
	if imageTag == "" {
		return fmt.Errorf("image tag is required")
	}

	args := []string{"build"}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, "-f", dockerfile, "-t", imageTag)
	args = append(args, dockerLabelArgs(labels)...)
	args = append(args, dockerBuildArgs()...)
	secretArgs, err := dockerSecretArgs(dockerfile)
	if err != nil {
		return err
	}
	args = append(args, secretArgs...)
	args = append(args, ".")
	if verbose {
		return runner.Run(ctx, contextDir, "docker", args...)
	}
	return runner.RunQuiet(ctx, contextDir, "docker", args...)
}

func pushDockerImage(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
	verbose bool,
) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if imageTag == "" {
		return fmt.Errorf("image tag is required")
	}
	if verbose {
		return runner.Run(ctx, contextDir, "docker", "push", imageTag)
	}
	return runner.RunQuiet(ctx, contextDir, "docker", "push", imageTag)
}

func dockerBuildArgs() []string {
	keys := []string{
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
		"http_proxy",
		"https_proxy",
		"no_proxy",
	}
	args := []string{}
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		args = append(args, "--build-arg", key+"="+value)
	}
	return args
}

func dockerSecretArgs(dockerfile string) ([]string, error) {
	if !needsRootCASecret(dockerfile) {
		return nil, nil
	}
	path, err := resolveRootCAPath()
	if err != nil {
		return nil, err
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
	return []string{"--secret", fmt.Sprintf("id=%s,src=%s", esbRootCASecretID, path)}, nil
}

func needsRootCASecret(dockerfile string) bool {
	base := filepath.Base(dockerfile)
	return base == "Dockerfile.os-base" || base == "Dockerfile.python-base"
}

func resolveRootCAPath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("ESB_CA_CERT_PATH")); value != "" {
		return ensureRootCAPath(expandHome(value))
	}
	if value := strings.TrimSpace(os.Getenv("ESB_CERT_DIR")); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), esbRootCACertName))
	}
	if value := strings.TrimSpace(os.Getenv("CAROOT")); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), esbRootCACertName))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("root CA not found: %w", err)
	}
	return ensureRootCAPath(filepath.Join(home, ".esb", "certs", esbRootCACertName))
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

func dockerLabelArgs(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		value := strings.TrimSpace(labels[key])
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}
	return args
}

func writeFunctionDockerignore(contextDir, functionDir string) error {
	rel, err := filepath.Rel(contextDir, functionDir)
	if err != nil {
		return nil
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
		"# Auto-generated by esb build.",
		"# What: Limit Docker build context to the active function and its layers.",
		"# Why: Reduce context upload size when using output_dir as build context.",
		"*",
		"!.dockerignore",
		"!functions/",
		"!" + rel + "/",
		"!" + rel + "/**",
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(contextDir, ".dockerignore"), []byte(content), 0o644)
}
