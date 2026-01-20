// Where: cli/internal/helpers/project_config.go
// What: Project configuration accessor for CLI commands.
// Why: Share generator.yml metadata and loader helpers across the command layer.
package helpers

import (
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// ProjectConfig holds metadata parsed from generator.yml.
type ProjectConfig struct {
	Name          string
	Dir           string
	GeneratorPath string
	Generator     config.GeneratorConfig
}

// ProjectConfigLoader loads generator metadata for the given project directory.
type ProjectConfigLoader func(projectDir string) (ProjectConfig, error)

// DefaultProjectConfigLoader returns the stock loader that reads generator.yml
// from the project directory and parses it into ProjectConfig.
func DefaultProjectConfigLoader() ProjectConfigLoader {
	return func(projectDir string) (ProjectConfig, error) {
		absDir, err := filepath.Abs(projectDir)
		if err != nil {
			return ProjectConfig{}, err
		}
		generatorPath := filepath.Join(absDir, "generator.yml")
		cfg, err := config.LoadGeneratorConfig(generatorPath)
		if err != nil {
			return ProjectConfig{}, err
		}
		name := strings.TrimSpace(cfg.App.Name)
		if name == "" {
			name = filepath.Base(absDir)
		}
		return ProjectConfig{
			Name:          name,
			Dir:           absDir,
			GeneratorPath: generatorPath,
			Generator:     cfg,
		}, nil
	}
}
