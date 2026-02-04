// Where: cli/internal/infra/build/go_builder.go
// What: Go-native deploy implementation for CLI deploy.
// Why: Build function images only during deploy. Control plane images are built
//
//	separately via docker compose up (which auto-builds if images don't exist).
package build

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/meta"
)

// PortDiscoverer defines the interface for discovering dynamically assigned ports.
type PortDiscoverer interface {
	Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error)
}

type GoBuilder struct {
	Runner         compose.CommandRunner
	ComposeRunner  compose.CommandRunner
	PortDiscoverer PortDiscoverer
	BuildCompose   func(ctx context.Context, runner compose.CommandRunner, opts compose.BuildOptions) error
	Generate       func(cfg config.GeneratorConfig, opts GenerateOptions) ([]template.FunctionSpec, error)
	FindRepoRoot   func(start string) (string, error)
}

func NewGoBuilder(discoverer PortDiscoverer) *GoBuilder {
	return &GoBuilder{
		Runner:         compose.ExecRunner{},
		ComposeRunner:  compose.ExecRunner{},
		PortDiscoverer: discoverer,
		BuildCompose:   compose.BuildProject,
		Generate:       GenerateFiles,
		FindRepoRoot:   findRepoRoot,
	}
}

func (b *GoBuilder) Build(request BuildRequest) error {
	if b == nil {
		return fmt.Errorf("builder is nil")
	}
	if request.ProjectDir == "" {
		return fmt.Errorf("project dir is required")
	}
	if request.TemplatePath == "" {
		return fmt.Errorf("template path is required")
	}
	if request.Env == "" {
		return fmt.Errorf("env is required")
	}
	if strings.TrimSpace(request.Mode) == "" {
		return fmt.Errorf("mode is required")
	}
	if strings.TrimSpace(request.Tag) == "" {
		return fmt.Errorf("tag is required")
	}
	if b.Runner == nil {
		return fmt.Errorf("runner is nil")
	}
	if b.ComposeRunner == nil {
		return fmt.Errorf("compose runner is nil")
	}
	if b.Generate == nil {
		return fmt.Errorf("generator is not configured")
	}
	if b.FindRepoRoot == nil {
		return fmt.Errorf("repo root finder is not configured")
	}

	templatePath, err := resolveTemplatePath(request.TemplatePath, request.ProjectDir)
	if err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	cfg := config.GeneratorConfig{
		App: config.AppConfig{
			Name: strings.TrimSpace(request.ProjectName),
		},
		Paths: config.PathsConfig{
			SamTemplate: templatePath,
			OutputDir:   strings.TrimSpace(request.OutputDir),
		},
	}
	if err := applyModeFromRequest(request.Mode); err != nil {
		return err
	}

	repoRoot, err := b.FindRepoRoot(request.ProjectDir)
	if err != nil {
		return err
	}

	mode := strings.TrimSpace(request.Mode)
	registry, err := resolveRegistryConfig()
	if err != nil {
		return err
	}
	imageTag := strings.TrimSpace(request.Tag)
	includeDockerOutput := !strings.EqualFold(mode, compose.ModeContainerd)

	artifactBase := resolveOutputDir(cfg.Paths.OutputDir, filepath.Dir(templatePath))
	cfg.Paths.OutputDir = filepath.Join(artifactBase, request.Env)

	composeProject := strings.TrimSpace(request.ProjectName)
	if composeProject == "" {
		brandName := strings.ToLower(cfg.App.Name)
		if brandName == "" {
			brandName = strings.ToLower(os.Getenv("CLI_CMD"))
		}
		if brandName == "" {
			brandName = meta.Slug
		}
		composeProject = fmt.Sprintf("%s-%s", brandName, strings.ToLower(request.Env))
	}

	if err := applyBuildEnv(request.Env, templatePath, composeProject); err != nil {
		return err
	}
	_ = os.Setenv("META_MODULE_CONTEXT", filepath.Join(repoRoot, "meta"))
	imageLabels := brandingImageLabels(composeProject, request.Env)
	rootFingerprint, err := resolveRootCAFingerprint()
	if err != nil {
		return err
	}
	if os.Getenv(constants.BuildArgCAFingerprint) == "" {
		_ = os.Setenv(constants.BuildArgCAFingerprint, rootFingerprint)
	}
	baseImageLabels := map[string]string{
		compose.ESBManagedLabel:       "true",
		compose.ESBCAFingerprintLabel: rootFingerprint,
	}

	if request.Verbose {
		fmt.Println("Generating files...")
		fmt.Printf("Using Template: %s\n", templatePath)
		fmt.Printf("Output Dir: %s\n", cfg.Paths.OutputDir)
		fmt.Println("Parameters:")
		for k, v := range cfg.Parameters {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}

	if !request.Verbose {
		fmt.Print("Generating files... ")
	}

	cfg.Parameters = toAnyMap(defaultGeneratorParameters())
	for key, value := range request.Parameters {
		cfg.Parameters[key] = value
	}
	runtimeRegistry := registry.Registry
	if value := strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)); value != "" {
		if !strings.HasSuffix(value, "/") {
			value += "/"
		}
		runtimeRegistry = value
	}
	registryForPush := registry.Registry
	builderNetworkMode := ""
	if registryForPush != "" {
		registryHost := resolveRegistryHost(registryForPush)
		isLocal := isLocalRegistryHost(registryHost)
		if isLocal {
			hostRegistryAddr, explicitHostAddr := resolveHostRegistryAddress()
			if strings.EqualFold(registryHost, "registry") {
				// Buildx needs host networking for external pulls; push via host-mapped registry port.
				builderNetworkMode = "host"
				registryForPush = fmt.Sprintf("%s/", hostRegistryAddr)
			} else {
				builderNetworkMode = "host"
			}
			if b.PortDiscoverer != nil && !explicitHostAddr {
				ports, err := b.PortDiscoverer.Discover(
					context.Background(),
					repoRoot,
					composeProject,
					request.Mode,
				)
				if err != nil {
					return err
				}
				if port, ok := ports[constants.EnvPortRegistry]; ok && port > 0 {
					hostRegistryAddr = fmt.Sprintf("127.0.0.1:%d", port)
					if strings.EqualFold(registryHost, "registry") {
						registryForPush = fmt.Sprintf("127.0.0.1:%d/", port)
					}
					if strings.EqualFold(registryHost, "localhost") || registryHost == "127.0.0.1" {
						registryForPush = fmt.Sprintf("127.0.0.1:%d/", port)
					}
				}
			}
			if err := waitForRegistry(hostRegistryAddr, 30*time.Second); err != nil {
				return err
			}
		}
	}
	if err := ensureBuildxBuilder(
		context.Background(),
		b.Runner,
		repoRoot,
		buildxBuilderOptions{
			NetworkMode: builderNetworkMode,
			ConfigPath:  strings.TrimSpace(os.Getenv(constants.EnvBuildkitdConfig)),
		},
	); err != nil {
		return err
	}
	functions, err := b.Generate(cfg, GenerateOptions{
		ProjectRoot:     repoRoot,
		Registry:        runtimeRegistry,
		BuildRegistry:   registryForPush,
		RuntimeRegistry: runtimeRegistry,
		Tag:             imageTag,
		Parameters:      request.Parameters,
		Verbose:         request.Verbose,
	})
	if err != nil {
		if !request.Verbose {
			fmt.Println("Failed")
		}
		return err
	}
	if !request.Verbose {
		fmt.Println("Done")
	}

	if err := stageConfigFiles(cfg.Paths.OutputDir, repoRoot, templatePath, composeProject, request.Env); err != nil {
		return err
	}

	cacheRoot := bakeCacheRoot(cfg.Paths.OutputDir)

	lambdaBaseTag := lambdaBaseImageTag(registryForPush, imageTag)
	lambdaTags := []string{lambdaBaseTag}

	if err := withBuildLock("base-images", func() error {
		proxyArgs := dockerBuildArgMap()
		commonDir := filepath.Join(repoRoot, "services", "common")

		if !request.Verbose {
			fmt.Print("Building base image... ")
		}
		buildLambda := true
		buildOs := true
		buildPython := true

		osBaseTag := fmt.Sprintf("%s-os-base:latest", meta.ImagePrefix)
		if !request.NoCache && dockerImageHasLabelValue(context.Background(), b.Runner, repoRoot, osBaseTag, compose.ESBCAFingerprintLabel, rootFingerprint) {
			buildOs = false
			if request.Verbose {
				fmt.Println("Skipping OS base image build (already exists).")
			} else {
				fmt.Print("Building OS base image... ")
				fmt.Println("Skipped")
			}
		} else if !request.Verbose {
			fmt.Print("Building OS base image... ")
		}

		pythonBaseTag := fmt.Sprintf("%s-python-base:latest", meta.ImagePrefix)
		if !request.NoCache && dockerImageHasLabelValue(context.Background(), b.Runner, repoRoot, pythonBaseTag, compose.ESBCAFingerprintLabel, rootFingerprint) {
			buildPython = false
			if request.Verbose {
				fmt.Println("Skipping Python base image build (already exists).")
			} else {
				fmt.Print("Building Python base image... ")
				fmt.Println("Skipped")
			}
		} else if !request.Verbose {
			fmt.Print("Building Python base image... ")
		}

		lambdaTarget := bakeTarget{
			Name:    "lambda-base",
			Tags:    lambdaTags,
			Outputs: resolveBakeOutputs(registryForPush, true, includeDockerOutput),
			Labels:  imageLabels,
			Args:    proxyArgs,
			NoCache: request.NoCache,
		}

		if err := applyBakeLocalCache(&lambdaTarget, cacheRoot, "base/lambda"); err != nil {
			return err
		}
		baseTargets := []bakeTarget{lambdaTarget}
		rootCAPath := ""
		if buildOs || buildPython {
			path, err := resolveRootCAPath()
			if err != nil {
				return err
			}
			rootCAPath = path
		}
		if buildOs {
			osTarget := bakeTarget{
				Name:       "os-base",
				Context:    commonDir,
				Dockerfile: filepath.Join(commonDir, "Dockerfile.os-base"),
				Tags:       []string{osBaseTag},
				Outputs:    resolveBakeOutputs(registryForPush, false, includeDockerOutput),
				Labels:     baseImageLabels,
				Args: mergeStringMap(proxyArgs, map[string]string{
					constants.BuildArgCAFingerprint: rootFingerprint,
					"ROOT_CA_MOUNT_ID":              meta.RootCAMountID,
					"ROOT_CA_CERT_FILENAME":         meta.RootCACertFilename,
				}),
				Secrets: []string{fmt.Sprintf("id=%s,src=%s", meta.RootCAMountID, rootCAPath)},
				NoCache: request.NoCache,
			}
			if err := applyBakeLocalCache(&osTarget, cacheRoot, "base"); err != nil {
				return err
			}
			baseTargets = append(baseTargets, osTarget)
		}
		if buildPython {
			pythonTarget := bakeTarget{
				Name:       "python-base",
				Context:    commonDir,
				Dockerfile: filepath.Join(commonDir, "Dockerfile.python-base"),
				Tags:       []string{pythonBaseTag},
				Outputs:    resolveBakeOutputs(registryForPush, false, includeDockerOutput),
				Labels:     baseImageLabels,
				Args: mergeStringMap(proxyArgs, map[string]string{
					constants.BuildArgCAFingerprint: rootFingerprint,
					"ROOT_CA_MOUNT_ID":              meta.RootCAMountID,
					"ROOT_CA_CERT_FILENAME":         meta.RootCACertFilename,
				}),
				Secrets: []string{fmt.Sprintf("id=%s,src=%s", meta.RootCAMountID, rootCAPath)},
				NoCache: request.NoCache,
			}
			if err := applyBakeLocalCache(&pythonTarget, cacheRoot, "base"); err != nil {
				return err
			}
			baseTargets = append(baseTargets, pythonTarget)
		}

		if err := runBakeGroup(
			context.Background(),
			b.Runner,
			repoRoot,
			"esb-base",
			baseTargets,
			request.Verbose,
		); err != nil {
			if !request.Verbose {
				if buildLambda {
					fmt.Println("Failed")
				}
				if buildOs {
					fmt.Println("Failed")
				}
				if buildPython {
					fmt.Println("Failed")
				}
			}
			return err
		}

		if !request.Verbose {
			if buildLambda {
				fmt.Println("Done")
			}
			if buildOs {
				fmt.Println("Done")
			}
			if buildPython {
				fmt.Println("Done")
			}
		}
		return nil
	}); err != nil {
		return err
	}

	baseImageID := dockerImageID(context.Background(), b.Runner, repoRoot, lambdaBaseTag)
	imageFingerprint, err := buildImageFingerprint(
		cfg.Paths.OutputDir,
		composeProject,
		request.Env,
		baseImageID,
		functions,
	)
	if err != nil {
		return err
	}
	functionLabels := make(map[string]string, len(imageLabels)+2)
	for key, value := range imageLabels {
		functionLabels[key] = value
	}
	if imageFingerprint != "" {
		functionLabels[compose.ESBImageFingerprintLabel] = imageFingerprint
	}
	functionLabels[compose.ESBKindLabel] = "function"

	if !request.Verbose {
		fmt.Printf("Building function images (%d functions)...\n", len(functions))
	}
	if err := buildFunctionImages(
		context.Background(),
		b.Runner,
		repoRoot,
		cfg.Paths.OutputDir,
		functions,
		registryForPush,
		imageTag,
		request.NoCache,
		request.Verbose,
		functionLabels,
		cacheRoot,
		includeDockerOutput,
	); err != nil {
		if !request.Verbose {
			fmt.Printf("Building function images (%d functions)... Failed\n", len(functions))
		}
		return err
	}
	if !request.Verbose {
		fmt.Printf("Building function images (%d functions)... Done\n", len(functions))
	}
	// Control plane images are now built separately via `esb build-infra` or docker compose
	// Only function images are built during deploy
	if request.Bundle {
		manifestPath, err := writeBundleManifest(
			context.Background(),
			bundleManifestInput{
				RepoRoot:        repoRoot,
				OutputDir:       cfg.Paths.OutputDir,
				TemplatePath:    templatePath,
				Parameters:      cfg.Parameters,
				Project:         composeProject,
				Env:             request.Env,
				Mode:            request.Mode,
				ImageTag:        imageTag,
				Registry:        registryForPush,
				ServiceRegistry: registry.Registry,
				Functions:       functions,
				Runner:          b.Runner,
			},
		)
		if err != nil {
			return err
		}
		if request.Verbose {
			fmt.Printf("Bundle manifest written: %s\n", manifestPath)
		}
	}
	return nil
}

func isLocalRegistryHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "registry", "localhost", "127.0.0.1":
		return true
	default:
		return false
	}
}

func resolveRegistryHost(registry string) string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	if slash := strings.Index(trimmed, "/"); slash != -1 {
		trimmed = trimmed[:slash]
	}
	host := trimmed
	if colon := strings.Index(host, ":"); colon != -1 {
		host = host[:colon]
	}
	return host
}

func resolveHostRegistryAddress() (string, bool) {
	if value := strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")); value != "" {
		return strings.TrimPrefix(value, "http://"), true
	}
	port := strings.TrimSpace(os.Getenv(constants.EnvPortRegistry))
	if port == "" {
		port = "5010"
	}
	return fmt.Sprintf("127.0.0.1:%s", port), false
}

func waitForRegistry(registry string, timeout time.Duration) error {
	if strings.TrimSpace(os.Getenv("ESB_REGISTRY_WAIT")) == "0" {
		return nil
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(registry), "/")
	if trimmed == "" {
		return nil
	}
	url := fmt.Sprintf("http://%s/v2/", trimmed)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create registry request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("registry not responding at %s", url)
}
