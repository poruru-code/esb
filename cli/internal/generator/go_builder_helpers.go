// Where: cli/internal/generator/go_builder_helpers.go
// What: Helper utilities for GoBuilder image builds and staging.
// Why: Keep GoBuilder focused on orchestration logic.
package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/poruru/edge-serverless-box/meta"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

type registryConfig struct {
	Registry string
}

func resolveRegistryConfig(mode string) (registryConfig, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		return registryConfig{}, err
	}
	registry := strings.TrimSpace(os.Getenv(key))
	if registry == "" {
		if normalized == compose.ModeDocker {
			registry = constants.DefaultContainerRegistryHost
		} else {
			registry = constants.DefaultContainerRegistry
		}
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
	destDir := staging.ConfigDir(composeProject, env)
	if err := removeDir(destDir); err != nil {
		return err
	}
	if err := ensureDir(destDir); err != nil {
		return err
	}

	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml"} {
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
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
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
	functions []FunctionSpec,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
	cacheRoot string,
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
				Outputs:    resolveBakeOutputs(registry, true),
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

func buildControlImages(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	outputDir string,
	mode string,
	registry string,
	tag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
	cacheRoot string,
) ([]string, error) {
	if runner == nil {
		return nil, fmt.Errorf("command runner is nil")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return nil, fmt.Errorf("repo root is required")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = compose.ModeDocker
	}

	configDir := filepath.ToSlash(filepath.Join(outputDir, "config"))
	gatewayDir := filepath.ToSlash(filepath.Join(root, "services", "gateway"))
	agentDir := filepath.ToSlash(filepath.Join(root, "services", "agent"))
	provisionerDir := filepath.ToSlash(filepath.Join(root, "services", "provisioner"))
	runtimeNodeDir := filepath.ToSlash(filepath.Join(root, "services", "runtime-node"))

	pythonBaseContext := "target:python-base"
	osBaseContext := "target:os-base"

	serviceUser := meta.Slug
	serviceUID := strings.TrimSpace(os.Getenv("RUN_UID"))
	if serviceUID == "" {
		serviceUID = "1000"
	}
	serviceGID := strings.TrimSpace(os.Getenv("RUN_GID"))
	if serviceGID == "" {
		serviceGID = "1000"
	}

	proxyArgs := dockerBuildArgMap()
	makeTag := func(name string) string {
		return joinRegistry(registry, fmt.Sprintf("%s:%s", name, tag))
	}

	rootFingerprint, err := resolveRootCAFingerprint()
	if err != nil {
		return nil, err
	}
	rootCAPath, err := resolveRootCAPath()
	if err != nil {
		return nil, err
	}
	baseImageLabels := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		baseImageLabels[key] = value
	}
	baseImageLabels[compose.ESBCAFingerprintLabel] = rootFingerprint

	osBaseTag := fmt.Sprintf("%s-os-base:latest", meta.ImagePrefix)
	osTarget := bakeTarget{
		Name:    "os-base",
		Tags:    []string{osBaseTag},
		Outputs: resolveBakeOutputs(registry, false),
		Labels:  baseImageLabels,
		Args: mergeStringMap(proxyArgs, map[string]string{
			constants.BuildArgCAFingerprint: rootFingerprint,
			"ROOT_CA_MOUNT_ID":              meta.RootCAMountID,
			"ROOT_CA_CERT_FILENAME":         meta.RootCACertFilename,
		}),
		Secrets: []string{fmt.Sprintf("id=%s,src=%s", meta.RootCAMountID, rootCAPath)},
		NoCache: noCache,
	}
	if err := applyBakeLocalCache(&osTarget, cacheRoot, "base/os"); err != nil {
		return nil, err
	}

	pythonBaseTag := fmt.Sprintf("%s-python-base:latest", meta.ImagePrefix)
	pythonTarget := bakeTarget{
		Name:    "python-base",
		Tags:    []string{pythonBaseTag},
		Outputs: resolveBakeOutputs(registry, false),
		Labels:  baseImageLabels,
		Args: mergeStringMap(proxyArgs, map[string]string{
			constants.BuildArgCAFingerprint: rootFingerprint,
			"ROOT_CA_MOUNT_ID":              meta.RootCAMountID,
			"ROOT_CA_CERT_FILENAME":         meta.RootCACertFilename,
		}),
		Secrets: []string{fmt.Sprintf("id=%s,src=%s", meta.RootCAMountID, rootCAPath)},
		NoCache: noCache,
	}
	if err := applyBakeLocalCache(&pythonTarget, cacheRoot, "base/python"); err != nil {
		return nil, err
	}

	targets := []bakeTarget{osTarget, pythonTarget}
	built := make([]string, 0, 4)

	switch mode {
	case compose.ModeContainerd:
		runtimeDockerfile := filepath.Join(runtimeNodeDir, "Dockerfile.containerd")
		if _, err := os.Stat(runtimeDockerfile); err != nil {
			return nil, fmt.Errorf("dockerfile not found: %w", err)
		}
		runtimeTag := makeTag(fmt.Sprintf("%s-runtime-node-containerd", meta.ImagePrefix))
		runtimeTarget := bakeTarget{
			Name:    "runtime-node-containerd",
			Tags:    []string{runtimeTag},
			Outputs: resolveBakeOutputs(registry, true),
			Labels:  labels,
			Args: mergeStringMap(proxyArgs, map[string]string{
				"OS_BASE_IMAGE": "os-base",
			}),
			Contexts: map[string]string{
				"os-base": osBaseContext,
			},
			NoCache: noCache,
		}
		if err := applyBakeLocalCache(&runtimeTarget, cacheRoot, "control"); err != nil {
			return nil, err
		}
		targets = append(targets, runtimeTarget)
		built = append(built, "runtime-node")
	}

	agentDockerfile := filepath.Join(agentDir, fmt.Sprintf("Dockerfile.%s", mode))
	if _, err := os.Stat(agentDockerfile); err != nil {
		return nil, fmt.Errorf("dockerfile not found: %w", err)
	}
	agentTag := makeTag(fmt.Sprintf("%s-agent-%s", meta.ImagePrefix, mode))
	agentTarget := bakeTarget{
		Name:    fmt.Sprintf("agent-%s", mode),
		Tags:    []string{agentTag},
		Outputs: resolveBakeOutputs(registry, true),
		Labels:  labels,
		Args: mergeStringMap(proxyArgs, map[string]string{
			"OS_BASE_IMAGE": "os-base",
		}),
		Contexts: map[string]string{
			"os-base": osBaseContext,
		},
		NoCache: noCache,
	}
	if err := applyBakeLocalCache(&agentTarget, cacheRoot, "control"); err != nil {
		return nil, err
	}
	targets = append(targets, agentTarget)
	built = append(built, "agent")

	provisionerDockerfile := filepath.Join(provisionerDir, "Dockerfile")
	if _, err := os.Stat(provisionerDockerfile); err != nil {
		return nil, fmt.Errorf("dockerfile not found: %w", err)
	}
	provisionerTag := makeTag(fmt.Sprintf("%s-provisioner", meta.ImagePrefix))
	provisionerTarget := bakeTarget{
		Name:    "provisioner",
		Tags:    []string{provisionerTag},
		Outputs: resolveBakeOutputs(registry, true),
		Labels:  labels,
		Args: mergeStringMap(proxyArgs, map[string]string{
			"PYTHON_BASE_IMAGE": "python-base",
		}),
		Contexts: map[string]string{
			"config":      configDir,
			"python-base": pythonBaseContext,
		},
		NoCache: noCache,
	}
	if err := applyBakeLocalCache(&provisionerTarget, cacheRoot, "control"); err != nil {
		return nil, err
	}
	targets = append(targets, provisionerTarget)
	built = append(built, "provisioner")

	gatewayDockerfile := filepath.Join(gatewayDir, fmt.Sprintf("Dockerfile.%s", mode))
	if _, err := os.Stat(gatewayDockerfile); err != nil {
		return nil, fmt.Errorf("dockerfile not found: %w", err)
	}
	gatewayTag := makeTag(fmt.Sprintf("%s-gateway-%s", meta.ImagePrefix, mode))
	gatewayTarget := bakeTarget{
		Name:    fmt.Sprintf("gateway-%s", mode),
		Tags:    []string{gatewayTag},
		Outputs: resolveBakeOutputs(registry, true),
		Labels:  labels,
		Args: mergeStringMap(proxyArgs, map[string]string{
			"PYTHON_BASE_IMAGE": "python-base",
			"SERVICE_USER":      serviceUser,
			"SERVICE_UID":       serviceUID,
			"SERVICE_GID":       serviceGID,
		}),
		Contexts: map[string]string{
			"config":      configDir,
			"python-base": pythonBaseContext,
		},
		NoCache: noCache,
	}
	if err := applyBakeLocalCache(&gatewayTarget, cacheRoot, "control"); err != nil {
		return nil, err
	}
	targets = append(targets, gatewayTarget)
	built = append(built, "gateway")

	if len(targets) == 0 {
		return nil, nil
	}
	if verbose {
		fmt.Println("Building control plane images...")
		for _, name := range built {
			fmt.Printf("  Building image for %s...\n", name)
		}
	}
	if err := runBakeGroup(
		ctx,
		runner,
		root,
		"esb-control",
		targets,
		verbose,
	); err != nil {
		return nil, err
	}

	return built, nil
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
	return os.WriteFile(filepath.Join(contextDir, ".dockerignore"), []byte(content), 0o644)
}
