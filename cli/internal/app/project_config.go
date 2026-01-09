// Where: cli/internal/app/project_config.go
// What: Project config loader for generator.yml.
// Why: Share project metadata between env/project commands.
package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type projectConfig struct {
	Name          string
	Dir           string
	GeneratorPath string
	Generator     config.GeneratorConfig
}

func loadProjectConfig(projectDir string) (projectConfig, error) {
	if strings.TrimSpace(projectDir) == "" {
		return projectConfig{}, fmt.Errorf("project dir is required")
	}
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return projectConfig{}, err
	}
	generatorPath := filepath.Join(absDir, "generator.yml")
	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		return projectConfig{}, err
	}
	name := strings.TrimSpace(cfg.App.Name)
	if name == "" {
		name = filepath.Base(absDir)
	}
	return projectConfig{
		Name:          name,
		Dir:           absDir,
		GeneratorPath: generatorPath,
		Generator:     cfg,
	}, nil
}
