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
)

type registryConfig struct {
	External string
	Internal string
}

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

func stageConfigFiles(outputDir, repoRoot, env string) error {
	configDir := filepath.Join(outputDir, "config")
	destDir := filepath.Join(repoRoot, "services", "gateway", ".esb-staging", env, "config")
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
	dockerfile := filepath.Join(assetsDir, "Dockerfile.base")
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("base dockerfile not found: %w", err)
	}

	imageTag := fmt.Sprintf("esb-lambda-base:%s", tag)
	if registry != "" {
		imageTag = fmt.Sprintf("%s/%s", registry, imageTag)
	}

	if err := buildDockerImage(ctx, runner, assetsDir, "Dockerfile.base", imageTag, noCache, labels); err != nil {
		return err
	}
	if registry != "" {
		return pushDockerImage(ctx, runner, assetsDir, imageTag)
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

		if err := buildDockerImage(ctx, runner, outputDir, dockerfileRel, imageTag, noCache, labels); err != nil {
			return err
		}
		if registry != "" {
			if err := pushDockerImage(ctx, runner, outputDir, imageTag); err != nil {
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
	services := map[string]string{
		"esb-runtime-node": filepath.Join(repoRoot, "services", "runtime-node"),
		"esb-agent":        filepath.Join(repoRoot, "services", "agent"),
	}
	for name, dir := range services {
		dockerfile := filepath.Join(dir, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			return fmt.Errorf("service dockerfile not found: %w", err)
		}
		imageTag := fmt.Sprintf("%s:%s", name, tag)
		if registry != "" {
			imageTag = fmt.Sprintf("%s/%s", registry, imageTag)
		}
		if err := buildDockerImage(ctx, runner, dir, "Dockerfile", imageTag, noCache, labels); err != nil {
			return err
		}
		if registry != "" {
			if err := pushDockerImage(ctx, runner, dir, imageTag); err != nil {
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
	args = append(args, ".")
	return runner.Run(ctx, contextDir, "docker", args...)
}

func pushDockerImage(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if imageTag == "" {
		return fmt.Errorf("image tag is required")
	}
	return runner.Run(ctx, contextDir, "docker", "push", imageTag)
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
