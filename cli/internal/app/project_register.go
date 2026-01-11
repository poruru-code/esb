// Where: cli/internal/app/project_register.go
// What: Project registration after init.
// Why: Persist project metadata into global config for later selection.
package app

import (
	"path/filepath"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// registerProject adds a project to the global configuration after init.
// It persists its path and last-used timestamp.
func registerProject(generatorPath string, deps Dependencies) error {
	projectDir := filepath.Dir(generatorPath)
	project, err := loadProjectConfig(projectDir)
	if err != nil {
		return err
	}

	path, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}
	cfg, err := loadGlobalConfig(path)
	if err != nil {
		return err
	}

	updated := normalizeGlobalConfig(cfg)
	entry := updated.Projects[project.Name]
	entry.Path = project.Dir
	entry.LastUsed = now(deps).Format(time.RFC3339)
	updated.Projects[project.Name] = entry

	return saveGlobalConfig(path, updated)
}
