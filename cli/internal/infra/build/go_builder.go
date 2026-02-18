// Where: cli/internal/infra/build/go_builder.go
// What: Go-native deploy implementation for CLI deploy.
// Why: Keep Build focused on orchestration while delegating per-phase details.
package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	templategen "github.com/poruru/edge-serverless-box/cli/internal/infra/templategen"
)

// PortDiscoverer defines the interface for discovering dynamically assigned ports.
type PortDiscoverer interface {
	Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error)
}

type GoBuilder struct {
	Runner              compose.CommandRunner
	ComposeRunner       compose.CommandRunner
	PortDiscoverer      PortDiscoverer
	Out                 io.Writer
	BuildCompose        func(ctx context.Context, runner compose.CommandRunner, opts compose.BuildOptions) error
	Generate            func(cfg config.GeneratorConfig, opts templategen.GenerateOptions) ([]template.FunctionSpec, error)
	WriteBundleManifest func(ctx context.Context, input templategen.BundleManifestInput) (string, error)
	FindRepoRoot        func(start string) (string, error)
}

func NewGoBuilder(discoverer PortDiscoverer) *GoBuilder {
	return &GoBuilder{
		Runner:              compose.ExecRunner{},
		ComposeRunner:       compose.ExecRunner{},
		PortDiscoverer:      discoverer,
		Out:                 os.Stdout,
		BuildCompose:        compose.BuildProject,
		Generate:            templategen.GenerateFiles,
		WriteBundleManifest: templategen.WriteBundleManifest,
		FindRepoRoot:        findRepoRoot,
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
	out := resolveBuildOutput(b.Out)

	templatePath, err := templategen.ResolveTemplatePath(request.TemplatePath, request.ProjectDir)
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
	imageTag := strings.TrimSpace(request.Tag)
	includeDockerOutput := !strings.EqualFold(mode, compose.ModeContainerd)

	artifactBase := templategen.ResolveOutputDir(cfg.Paths.OutputDir, filepath.Dir(templatePath))
	cfg.Paths.OutputDir = filepath.Join(artifactBase, request.Env)
	lockRoot := ""
	if request.BuildImages {
		lockRoot, err = staging.RootDir(templatePath)
		if err != nil {
			return err
		}
	}

	composeProject := resolveComposeProjectName(request.ProjectName, cfg.App.Name, request.Env)
	if err := applyBuildEnv(request.Env, templatePath, composeProject, request.SkipStaging); err != nil {
		return err
	}
	imageLabels := brandingImageLabels(composeProject, request.Env)

	cfg.Parameters = toAnyMap(defaultGeneratorParameters())
	for key, value := range request.Parameters {
		cfg.Parameters[key] = value
	}
	phase := newPhaseReporter(request.Verbose, request.Emoji, out)
	if request.Verbose {
		_, _ = fmt.Fprintln(out, "Generating files...")
		_, _ = fmt.Fprintf(out, "Using Template: %s\n", templatePath)
		_, _ = fmt.Fprintf(out, "Output Dir: %s\n", cfg.Paths.OutputDir)
		_, _ = fmt.Fprintln(out, "Parameters:")
		for _, key := range sortedAnyKeys(cfg.Parameters) {
			_, _ = fmt.Fprintf(out, "  %s: %v\n", key, cfg.Parameters[key])
		}
	}

	registryInfo, err := resolveGenerateRegistryInfo()
	if err != nil {
		return err
	}
	if request.BuildImages {
		registryInfo, err = b.resolveBuildRegistryInfo(
			context.Background(),
			repoRoot,
			composeProject,
			request,
		)
		if err != nil {
			return err
		}
		if err := ensureBuildxBuilder(
			context.Background(),
			b.Runner,
			repoRoot,
			lockRoot,
			buildxBuilderOptions{
				NetworkMode: registryInfo.BuilderNetworkMode,
				ConfigPath:  strings.TrimSpace(os.Getenv(constants.EnvBuildkitdConfig)),
			},
		); err != nil {
			return err
		}
	}

	var functions []template.FunctionSpec
	if err := phase.Run("Generate config", func() error {
		generated, err := b.generateAndStageConfig(
			cfg,
			templategen.GenerateOptions{
				ProjectRoot:     repoRoot,
				Out:             out,
				Registry:        registryInfo.RuntimeRegistry,
				BuildRegistry:   registryInfo.PushRegistry,
				RuntimeRegistry: registryInfo.RuntimeRegistry,
				Tag:             imageTag,
				Parameters:      request.Parameters,
				ImageSources:    request.ImageSources,
				ImageRuntimes:   request.ImageRuntimes,
				Verbose:         request.Verbose,
			},
			repoRoot,
			templatePath,
			composeProject,
			request.Env,
			request.SkipStaging,
		)
		if err != nil {
			return err
		}
		functions = generated
		return nil
	}); err != nil {
		return err
	}

	if !request.BuildImages {
		if request.Bundle {
			return fmt.Errorf("bundle manifest requires image builds")
		}
		if request.Verbose {
			_, _ = fmt.Fprintln(out, "Skipping image build phase (render-only)")
		}
		return nil
	}

	rootFingerprint, err := resolveRootCAFingerprint()
	if err != nil {
		return err
	}
	if os.Getenv(constants.BuildArgCAFingerprint) == "" {
		_ = os.Setenv(constants.BuildArgCAFingerprint, rootFingerprint)
	}

	lambdaBaseTag := lambdaBaseImageTag(registryInfo.PushRegistry, imageTag)
	if err := phase.Run("Build base images", func() error {
		return b.buildBaseImages(baseImageBuildInput{
			RepoRoot:            repoRoot,
			LockRoot:            lockRoot,
			RegistryForPush:     registryInfo.PushRegistry,
			ImageTag:            imageTag,
			ImageLabels:         imageLabels,
			RootFingerprint:     rootFingerprint,
			NoCache:             request.NoCache,
			Verbose:             request.Verbose,
			IncludeDockerOutput: includeDockerOutput,
			LambdaBaseTag:       lambdaBaseTag,
			Out:                 out,
		})
	}); err != nil {
		return err
	}

	baseImageID := dockerImageID(context.Background(), b.Runner, repoRoot, lambdaBaseTag)
	imageSourceDigests := map[string]string{}
	if !request.NoCache {
		imageSourceDigests, err = resolveImageSourceDigests(
			context.Background(),
			b.Runner,
			repoRoot,
			functions,
			request.Verbose,
			out,
		)
		if err != nil {
			return err
		}
	}
	imageFingerprint, err := buildImageFingerprint(
		cfg.Paths.OutputDir,
		composeProject,
		request.Env,
		baseImageID,
		functions,
		imageSourceDigests,
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

	buildCount := 0
	for range functions {
		buildCount++
	}
	label := fmt.Sprintf("Build function images (%d)", buildCount)
	if err := phase.Run(label, func() error {
		return buildFunctionImages(
			context.Background(),
			b.Runner,
			repoRoot,
			lockRoot,
			cfg.Paths.OutputDir,
			functions,
			registryInfo.PushRegistry,
			imageTag,
			request.NoCache,
			request.Verbose,
			functionLabels,
			includeDockerOutput,
			out,
		)
	}); err != nil {
		return err
	}

	// Control plane images are now built separately via `esb build-infra` or docker compose.
	// Only function images are built during deploy.
	if request.Bundle {
		if b.WriteBundleManifest == nil {
			return fmt.Errorf("bundle manifest writer is not configured")
		}
		manifestPath, err := b.WriteBundleManifest(
			context.Background(),
			templategen.BundleManifestInput{
				RepoRoot:        repoRoot,
				OutputDir:       cfg.Paths.OutputDir,
				TemplatePath:    templatePath,
				Parameters:      cfg.Parameters,
				Project:         composeProject,
				Env:             request.Env,
				Mode:            request.Mode,
				ImageTag:        imageTag,
				Registry:        registryInfo.PushRegistry,
				ServiceRegistry: registryInfo.ServiceRegistry,
				Functions:       functions,
				Runner:          b.Runner,
			},
		)
		if err != nil {
			return err
		}
		if request.Verbose {
			_, _ = fmt.Fprintf(out, "Bundle manifest written: %s\n", manifestPath)
		}
	}
	return nil
}

func sortedAnyKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
