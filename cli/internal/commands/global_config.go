// Where: cli/internal/commands/global_config.go
// What: Global config helpers for env/project commands.
// Why: Centralize ~/.esb/config.yaml handling and defaults.
package commands

import (
	"os"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// loadGlobalConfig loads the global configuration from the specified path.
// Returns a default config if the file doesn't exist.
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

// defaultGlobalConfig returns an empty but properly initialized GlobalConfig.
func defaultGlobalConfig() config.GlobalConfig {
	return config.DefaultGlobalConfig()
}

// normalizeGlobalConfig ensures all map fields are initialized and the
// version field is set. Prevents nil pointer dereferences.
func normalizeGlobalConfig(cfg config.GlobalConfig) config.GlobalConfig {
	defaults := config.DefaultGlobalConfig()
	if cfg.Version == 0 {
		cfg.Version = defaults.Version
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]config.ProjectEntry{}
	}
	return cfg
}

// now returns the current time using the injected Now function from deps,
// or time.Now() if not configured. Enables time mocking in tests.
func now(deps Dependencies) time.Time {
	if deps.Now != nil {
		return deps.Now()
	}
	return time.Now()
}
