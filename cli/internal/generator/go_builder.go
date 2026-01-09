// Where: cli/internal/generator/go_builder.go
// What: Go-native build implementation for esb build.
// Why: Replace the Python build pipeline with a Go-based generator + docker workflow.
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type GoBuilder struct {
	Runner        compose.CommandRunner
	ComposeRunner compose.CommandRunner
	BuildCompose  func(ctx context.Context, runner compose.CommandRunner, opts compose.BuildOptions) error
	Generate      func(cfg config.GeneratorConfig, opts GenerateOptions) ([]FunctionSpec, error)
	FindRepoRoot  func(start string) (string, error)
}

func NewGoBuilder() *GoBuilder {
	return &GoBuilder{
		Runner:        compose.ExecRunner{},
		ComposeRunner: compose.ExecRunner{},
		BuildCompose:  compose.BuildProject,
		Generate:      GenerateFiles,
		FindRepoRoot:  findRepoRoot,
	}
}

func (b *GoBuilder) Build(request app.BuildRequest) error {
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

	generatorPath := filepath.Join(request.ProjectDir, "generator.yml")
	if _, err := os.Stat(generatorPath); err != nil {
		return fmt.Errorf("generator.yml not found: %w", err)
	}

	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		return fmt.Errorf("read generator.yml: %w", err)
	}
	if !cfg.Environments.Has(request.Env) {
		return fmt.Errorf("environment not registered: %s", request.Env)
	}
	applyModeFromConfig(cfg, request.Env)

	repoRoot, err := b.FindRepoRoot(request.ProjectDir)
	if err != nil {
		return err
	}

	mode, _ := cfg.Environments.Mode(request.Env)
	registry := resolveRegistryConfig(mode)
	imageTag := resolveImageTag(request.Env)

	cfg.Paths.SamTemplate = templatePath
	outputBase, err := resolveOutputDir(cfg.Paths.OutputDir, filepath.Dir(templatePath))
	if err != nil {
		return err
	}
	cfg.Paths.OutputDir = filepath.Join(outputBase, request.Env)

	functions, err := b.Generate(cfg, GenerateOptions{
		ProjectRoot:      repoRoot,
		RegistryExternal: registry.External,
		RegistryInternal: registry.Internal,
		Tag:              imageTag,
		Parameters:       defaultGeneratorParameters(),
	})
	if err != nil {
		return err
	}

	if err := stageConfigFiles(cfg.Paths.OutputDir, repoRoot, request.Env); err != nil {
		return err
	}
	applyBuildEnv(request.Env)

	if registry.External != "" {
		if err := ensureRegistryRunning(
			context.Background(),
			b.ComposeRunner,
			repoRoot,
			fmt.Sprintf("esb-%s", strings.ToLower(request.Env)),
			mode,
		); err != nil {
			return err
		}
	}

	if err := buildBaseImage(context.Background(), b.Runner, repoRoot, registry.External, imageTag, request.NoCache); err != nil {
		return err
	}
	if err := buildFunctionImages(
		context.Background(),
		b.Runner,
		cfg.Paths.OutputDir,
		functions,
		registry.External,
		imageTag,
		request.NoCache,
	); err != nil {
		return err
	}
	if strings.EqualFold(mode, compose.ModeFirecracker) {
		if err := buildServiceImages(context.Background(), b.Runner, repoRoot, registry.External, imageTag, request.NoCache); err != nil {
			return err
		}
	}

	opts := compose.BuildOptions{
		RootDir:  repoRoot,
		Project:  fmt.Sprintf("esb-%s", strings.ToLower(request.Env)),
		Mode:     mode,
		Target:   "control",
		Services: []string{"gateway", "agent"},
		NoCache:  request.NoCache,
	}
	return b.BuildCompose(context.Background(), b.ComposeRunner, opts)
}
