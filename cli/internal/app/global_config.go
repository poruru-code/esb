// Where: cli/internal/app/global_config.go
// What: Global config helpers for env/project commands.
// Why: Centralize ~/.esb/config.yaml handling and defaults.
package app

import (
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func resolveEnv(cli CLI, deps Dependencies) string {
	if strings.TrimSpace(cli.EnvFlag) != "" {
		return strings.TrimSpace(cli.EnvFlag)
	}

	path, err := config.GlobalConfigPath()
	if err != nil {
		return "default"
	}
	cfg, err := loadGlobalConfig(path)
	if err != nil {
		return "default"
	}
	if cfg.ActiveProject != "" {
		if env := strings.TrimSpace(cfg.ActiveEnvironments[cfg.ActiveProject]); env != "" {
			if deps.ProjectDir != "" {
				project, err := loadProjectConfig(deps.ProjectDir)
				if err == nil {
					if project.Generator.Environments.Has(env) {
						return env
					}
					return "default"
				}
			}
			return env
		}
	}
	return "default"
}

func loadGlobalConfig(path string) (config.GlobalConfig, error) {
	cfg, err := config.LoadGlobalConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultGlobalConfig(), nil
		}
		return config.GlobalConfig{}, err
	}
	return normalizeGlobalConfig(cfg), nil
}

func saveGlobalConfig(path string, cfg config.GlobalConfig) error {
	return config.SaveGlobalConfig(path, cfg)
}

func defaultGlobalConfig() config.GlobalConfig {
	return config.GlobalConfig{
		Version:            1,
		ActiveEnvironments: map[string]string{},
		Projects:           map[string]config.ProjectEntry{},
	}
}

func normalizeGlobalConfig(cfg config.GlobalConfig) config.GlobalConfig {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.ActiveEnvironments == nil {
		cfg.ActiveEnvironments = map[string]string{}
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]config.ProjectEntry{}
	}
	return cfg
}

func now(deps Dependencies) time.Time {
	if deps.Now != nil {
		return deps.Now()
	}
	return time.Now()
}
