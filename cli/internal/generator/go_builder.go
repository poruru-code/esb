// Where: cli/internal/generator/go_builder.go
// What: Go-native build implementation for CLI build.
// Why: Replace the Python build pipeline with a Go-based generator + docker workflow.
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/meta"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
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
	Generate       func(cfg config.GeneratorConfig, opts GenerateOptions) ([]FunctionSpec, error)
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
	if b.BuildCompose == nil {
		return fmt.Errorf("compose build is not configured")
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
	gitCtx, err := resolveGitContext(context.Background(), b.Runner, repoRoot)
	if err != nil {
		return err
	}
	traceTools, err := resolveTraceTools(repoRoot)
	if err != nil {
		return err
	}
	metaDir, err := prepareMetaContext(context.Background(), b.Runner, repoRoot, gitCtx, traceTools)
	if err != nil {
		return err
	}
	buildContexts := []buildContext{
		{Name: "meta", Path: metaDir},
	}

	mode := strings.TrimSpace(request.Mode)
	registry, err := resolveRegistryConfig(mode)
	if err != nil {
		return err
	}
	imageTag := strings.TrimSpace(request.Tag)

	outputBase, err := resolveOutputDir(cfg.Paths.OutputDir, filepath.Dir(templatePath))
	if err != nil {
		return err
	}
	cfg.Paths.OutputDir = filepath.Join(outputBase, request.Env)

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

	applyBuildEnv(request.Env, composeProject)
	_ = os.Setenv("META_CONTEXT", metaDir)
	_ = os.Setenv("META_MODULE_CONTEXT", filepath.Join(repoRoot, "meta"))
	imageLabels := brandingImageLabels(composeProject, request.Env)
	rootFingerprint, err := resolveRootCAFingerprint()
	if err != nil {
		return err
	}
	if os.Getenv(constants.BuildArgCAFingerprint) == "" {
		_ = os.Setenv(constants.BuildArgCAFingerprint, rootFingerprint)
	}
	baseImageLabels := make(map[string]string, len(imageLabels)+1)
	for key, value := range imageLabels {
		baseImageLabels[key] = value
	}
	baseImageLabels[compose.ESBCAFingerprintLabel] = rootFingerprint

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
	if request.Mode == compose.ModeContainerd {
		if value := strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)); value != "" {
			if !strings.HasSuffix(value, "/") {
				value += "/"
			}
			runtimeRegistry = value
		}
	}
	registryForPush := registry.Registry
	if request.Mode == compose.ModeContainerd && registryForPush != "" {
		trimmed := strings.TrimSuffix(registryForPush, "/")
		host := trimmed
		if slash := strings.Index(host, "/"); slash != -1 {
			host = host[:slash]
		}
		hostOnly := host
		if colon := strings.Index(hostOnly, ":"); colon != -1 {
			hostOnly = hostOnly[:colon]
		}
		if hostOnly == "registry" {
			if err := ensureRegistryRunning(
				context.Background(),
				b.ComposeRunner,
				repoRoot,
				composeProject,
				request.Mode,
			); err != nil {
				return err
			}
			if b.PortDiscoverer != nil {
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
					registryForPush = fmt.Sprintf("localhost:%d/", port)
				}
			}
		}
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

	if err := stageConfigFiles(cfg.Paths.OutputDir, repoRoot, composeProject, request.Env); err != nil {
		return err
	}

	if !request.Verbose {
		fmt.Print("Building base image... ")
	}
	lambdaBaseTag := lambdaBaseImageTag(registryForPush, imageTag)
	if err := buildBaseImage(
		context.Background(),
		b.Runner,
		repoRoot,
		registryForPush,
		imageTag,
		request.NoCache,
		request.Verbose,
		imageLabels,
		buildContexts,
	); err != nil {
		if !request.Verbose {
			fmt.Println("Failed")
		}
		return err
	}
	if !request.Verbose {
		fmt.Println("Done")
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
		fmt.Print("Building OS base image... ")
	}
	osBaseTag := fmt.Sprintf("%s-os-base:latest", meta.ImagePrefix)
	if err := withBuildLock("os-base", func() error {
		if !request.NoCache && dockerImageHasLabelValue(context.Background(), b.Runner, repoRoot, osBaseTag, compose.ESBCAFingerprintLabel, rootFingerprint) {
			if request.Verbose {
				fmt.Println("Skipping OS base image build (already exists).")
			} else {
				fmt.Println("Skipped")
			}
			return nil
		}
		if err := buildDockerImage(
			context.Background(),
			b.Runner,
			repoRoot,
			"services/common/Dockerfile.os-base",
			osBaseTag,
			request.NoCache,
			request.Verbose,
			baseImageLabels,
			buildContexts,
		); err != nil {
			if !request.Verbose {
				fmt.Println("Failed")
			}
			return err
		}
		if !request.Verbose {
			fmt.Println("Done")
		}
		return nil
	}); err != nil {
		return err
	}

	if !request.Verbose {
		fmt.Print("Building Python base image... ")
	}
	pythonBaseTag := fmt.Sprintf("%s-python-base:latest", meta.ImagePrefix)
	if err := withBuildLock("python-base", func() error {
		if !request.NoCache && dockerImageHasLabelValue(context.Background(), b.Runner, repoRoot, pythonBaseTag, compose.ESBCAFingerprintLabel, rootFingerprint) {
			if request.Verbose {
				fmt.Println("Skipping Python base image build (already exists).")
			} else {
				fmt.Println("Skipped")
			}
			return nil
		}
		if err := buildDockerImage(
			context.Background(),
			b.Runner,
			repoRoot,
			"services/common/Dockerfile.python-base",
			pythonBaseTag,
			request.NoCache,
			request.Verbose,
			baseImageLabels,
			buildContexts,
		); err != nil {
			if !request.Verbose {
				fmt.Println("Failed")
			}
			return err
		}
		if !request.Verbose {
			fmt.Println("Done")
		}
		return nil
	}); err != nil {
		return err
	}

	if !request.Verbose {
		fmt.Printf("Building function images (%d functions)...\n", len(functions))
	}
	if err := buildFunctionImages(
		context.Background(),
		b.Runner,
		cfg.Paths.OutputDir,
		functions,
		registryForPush,
		imageTag,
		request.NoCache,
		request.Verbose,
		functionLabels,
		buildContexts,
	); err != nil {
		if !request.Verbose {
			fmt.Printf("Building function images (%d functions)... Failed\n", len(functions))
		}
		return err
	}
	if !request.Verbose {
		fmt.Printf("Building function images (%d functions)... Done\n", len(functions))
	}
	if !request.Verbose {
		fmt.Println("Building control plane images...")
	}

	controlServices := []string{"os-base", "python-base", "gateway", "agent", "provisioner"}
	opts := compose.BuildOptions{
		RootDir:  repoRoot,
		Project:  composeProject,
		Mode:     mode,
		Target:   "control",
		Services: controlServices,
		NoCache:  request.NoCache,
		Verbose:  request.Verbose,
	}
	if err := b.BuildCompose(context.Background(), b.ComposeRunner, opts); err != nil {
		if !request.Verbose {
			fmt.Println("Building control plane images... Failed")
		}
		return err
	}
	if !request.Verbose {
		for _, svc := range controlServices {
			fmt.Printf("  - Built control plane image: %s\n", svc)
		}
		fmt.Println("Building control plane images... Done")
	}
	return nil
}
