// Where: cli/internal/commands/config_access.go
// What: Shared config loader helpers for commands.
// Why: Front-load config/FS dependencies via injected loaders.
package commands

import (
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
)

func globalConfigLoader(deps Dependencies) helpers.GlobalConfigLoader {
	if deps.GlobalConfigLoader != nil {
		return deps.GlobalConfigLoader
	}
	return helpers.DefaultGlobalConfigLoader()
}

func projectConfigLoader(deps Dependencies) helpers.ProjectConfigLoader {
	if deps.ProjectConfigLoader != nil {
		return deps.ProjectConfigLoader
	}
	return helpers.DefaultProjectConfigLoader()
}

func projectDirFinder(deps Dependencies) helpers.ProjectDirFinder {
	if deps.ProjectDirFinder != nil {
		return deps.ProjectDirFinder
	}
	return helpers.DefaultProjectDirFinder()
}

func loadGlobalConfigOrDefault(deps Dependencies) (config.GlobalConfig, error) {
	_, cfg, err := globalConfigLoader(deps)()
	return cfg, err
}

func loadGlobalConfigWithPath(deps Dependencies) (string, config.GlobalConfig, error) {
	return globalConfigLoader(deps)()
}

func loadProjectConfig(deps Dependencies, projectDir string) (helpers.ProjectConfig, error) {
	return projectConfigLoader(deps)(projectDir)
}
