// Where: tools-go/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/tools-go/internal/config"
)

type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
	NoCache      bool
}

type Builder interface {
	Build(request BuildRequest) error
}

func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Builder == nil {
		fmt.Fprintln(out, "build: not implemented")
		return 1
	}
	selection, err := resolveProjectSelection(cli, deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	projectDir := selection.Dir
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	generatorPath := filepath.Join(projectDir, "generator.yml")
	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	envDeps := deps
	envDeps.ProjectDir = projectDir
	env := resolveEnv(cli, envDeps)
	if !cfg.Environments.Has(env) {
		fmt.Fprintf(out, "environment not registered: %s\n", env)
		return 1
	}
	mode, _ := cfg.Environments.Mode(env)
	applyModeEnv(mode)
	applyEnvironmentDefaults(env, mode)
	templatePath := cfg.Paths.SamTemplate
	if selection.TemplateOverride != "" {
		templatePath = selection.TemplateOverride
	}
	if strings.TrimSpace(templatePath) == "" {
		fmt.Fprintln(out, "template is required")
		return 1
	}
	if !filepath.IsAbs(templatePath) {
		templatePath = filepath.Join(projectDir, templatePath)
	}
	templatePath = filepath.Clean(templatePath)
	if _, err := os.Stat(templatePath); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	request := BuildRequest{
		ProjectDir:   projectDir,
		TemplatePath: templatePath,
		Env:          env,
		NoCache:      cli.Build.NoCache,
	}

	if err := deps.Builder.Build(request); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "build complete")
	return 0
}
