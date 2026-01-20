// Where: cli/internal/commands/project_config.go
// What: Project config loader for generator.yml.
// Why: Share project metadata between env/project commands.
package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// projectConfig holds metadata about a project loaded from generator.yml.
// It includes the project name, directory path, and parsed configuration.
type projectConfig struct {
	Name          string
	Dir           string
	GeneratorPath string
	Generator     config.GeneratorConfig
}

// loadProjectConfig loads a project's configuration from the generator.yml
// file in the specified directory.
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
