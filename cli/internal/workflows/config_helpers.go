// Where: cli/internal/workflows/config_helpers.go
// What: Shared config helpers for workflows.
// Why: Keep normalization logic consistent across workflows.
package workflows

import "github.com/poruru/edge-serverless-box/cli/internal/config"

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
