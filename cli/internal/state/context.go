// Where: cli/internal/state/context.go
// What: Generator context resolution for state detection.
// Why: Normalize generator.yml inputs into canonical paths and metadata.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type Context struct {
	ProjectDir     string
	GeneratorPath  string
	TemplatePath   string
	OutputDir      string
	OutputEnvDir   string
	Env            string
	Mode           string
	ComposeProject string
}

func ResolveContext(projectDir, env string) (Context, error) {
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return Context{}, fmt.Errorf("resolve project dir: %w", err)
	}

	generatorPath := filepath.Join(absProjectDir, "generator.yml")
	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		return Context{}, fmt.Errorf("read generator.yml: %w", err)
	}

	if !cfg.Environments.Has(env) {
		return Context{}, fmt.Errorf("environment not registered: %s", env)
	}

	if cfg.Paths.SamTemplate == "" {
		return Context{}, fmt.Errorf("missing paths.sam_template")
	}

	templatePath := cfg.Paths.SamTemplate
	if !filepath.IsAbs(templatePath) {
		templatePath = filepath.Join(absProjectDir, templatePath)
	}
	templatePath = filepath.Clean(templatePath)
	if _, err := os.Stat(templatePath); err != nil {
		return Context{}, fmt.Errorf("template not found: %w", err)
	}

	outputDir := normalizeOutputDir(cfg.Paths.OutputDir)
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(absProjectDir, outputDir)
	}
	outputDir = filepath.Clean(outputDir)

	composeProject := fmt.Sprintf("esb-%s", strings.ToLower(env))
	mode, _ := cfg.Environments.Mode(env)

	return Context{
		ProjectDir:     absProjectDir,
		GeneratorPath:  generatorPath,
		TemplatePath:   templatePath,
		OutputDir:      outputDir,
		OutputEnvDir:   filepath.Join(outputDir, env),
		Env:            env,
		Mode:           mode,
		ComposeProject: composeProject,
	}, nil
}

func normalizeOutputDir(outputDir string) string {
	trimmed := strings.TrimRight(outputDir, "/\\")
	if trimmed == "" {
		return ".esb"
	}
	return trimmed
}
