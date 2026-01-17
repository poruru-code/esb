// Where: cli/internal/generator/build_env.go
// What: Environment helpers for generator build flows.
// Why: Keep build-related env setup shared without Python builder dependency.
package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

func applyBuildEnv(env, composeProject string) {
	configDir := staging.ConfigDirRelative(composeProject, env)
	_ = os.Setenv(constants.EnvESBConfigDir, filepath.ToSlash(configDir))
	if os.Getenv(constants.EnvESBProjectName) == "" {
		_ = os.Setenv(constants.EnvESBProjectName, staging.ComposeProjectKey(composeProject, env))
	}
	if os.Getenv(constants.EnvESBImageTag) == "" {
		_ = os.Setenv(constants.EnvESBImageTag, defaultImageTag(env))
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
}

func applyModeFromConfig(cfg config.GeneratorConfig, env string) {
	if strings.TrimSpace(os.Getenv(constants.EnvESBMode)) != "" {
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
	_ = os.Setenv(constants.EnvESBMode, strings.ToLower(mode))
}

func defaultImageTag(env string) string {
	mode := strings.TrimSpace(os.Getenv(constants.EnvESBMode))
	normalized := strings.ToLower(mode)
	switch normalized {
	case "docker":
		return "docker"
	case "containerd":
		return "containerd"
	case "firecracker":
		return "firecracker"
	}
	if strings.TrimSpace(env) != "" {
		return env
	}
	return "latest"
}

// findRepoRoot locates the repository root using shared config logic.
func findRepoRoot(start string) (string, error) {
	return config.ResolveRepoRoot(start)
}
