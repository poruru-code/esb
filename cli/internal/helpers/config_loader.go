// Where: cli/internal/helpers/config_loader.go
// What: Global configuration loader helpers.
// Why: Share config file access across commands.
package helpers

import (
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// GlobalConfigLoader loads the global CLI config and returns the path plus config.
type GlobalConfigLoader func() (string, config.GlobalConfig, error)

// DefaultGlobalConfigLoader returns the stock loader using config.GlobalConfigPath.
func DefaultGlobalConfigLoader() GlobalConfigLoader {
	return func() (string, config.GlobalConfig, error) {
		path, err := config.GlobalConfigPath()
		if err != nil {
			return "", config.GlobalConfig{}, err
		}
		cfg, err := config.LoadGlobalConfig(path)
		if err != nil {
			if os.IsNotExist(err) {
				return path, config.DefaultGlobalConfig(), nil
			}
			return path, config.GlobalConfig{}, err
		}
		return path, normalizeConfig(cfg), nil
	}
}

func normalizeConfig(cfg config.GlobalConfig) config.GlobalConfig {
	if cfg.Version == 0 {
		cfg.Version = config.DefaultGlobalConfig().Version
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]config.ProjectEntry{}
	}
	return cfg
}
