// Where: cli/internal/app/project_register.go
// What: Project registration after init.
// Why: Persist project metadata into global config for later selection.
package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// registerProject adds a project to the global configuration after init.
// It sets the project as active and persists its path and first environment.
func registerProject(generatorPath string, deps Dependencies) error {
	projectDir := filepath.Dir(generatorPath)
	project, err := loadProjectConfig(projectDir)
	if err != nil {
		return err
	}

	activeEnv := ""
	if len(project.Generator.Environments) > 0 {
		activeEnv = strings.TrimSpace(project.Generator.Environments[0].Name)
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
	updated.ActiveProject = project.Name
	if activeEnv != "" {
		updated.ActiveEnvironments[project.Name] = activeEnv
	}
	updated.Projects[project.Name] = config.ProjectEntry{
		Path:     project.Dir,
		LastUsed: now(deps).Format(time.RFC3339),
	}

	return saveGlobalConfig(path, updated)
}
