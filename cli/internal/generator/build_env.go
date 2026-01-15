// Where: cli/internal/generator/build_env.go
// What: Environment helpers for generator build flows.
// Why: Keep build-related env setup shared without Python builder dependency.
package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

func applyBuildEnv(env, composeProject string) {
	configDir := staging.ConfigDirRelative(composeProject, env)
	_ = os.Setenv("ESB_CONFIG_DIR", filepath.ToSlash(configDir))
	if os.Getenv("ESB_PROJECT_NAME") == "" {
		_ = os.Setenv("ESB_PROJECT_NAME", staging.ComposeProjectKey(composeProject, env))
	}
	if os.Getenv("ESB_IMAGE_TAG") == "" {
		_ = os.Setenv("ESB_IMAGE_TAG", env)
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
}

func applyModeFromConfig(cfg config.GeneratorConfig, env string) {
	if strings.TrimSpace(os.Getenv("ESB_MODE")) != "" {
		return
	}
	mode, ok := cfg.Environments.Mode(env)
	if !ok {
		return
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return
	}
	_ = os.Setenv("ESB_MODE", strings.ToLower(mode))
}

// findRepoRoot locates the ESB repository root using shared config logic.
func findRepoRoot(start string) (string, error) {
	return config.ResolveRepoRoot(start)
}
