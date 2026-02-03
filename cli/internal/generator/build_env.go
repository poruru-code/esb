// Where: cli/internal/generator/build_env.go
// What: Environment helpers for generator build flows.
// Why: Keep build-related env setup shared without Python builder dependency.
package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func applyBuildEnv(env, composeProject string) {
	configDir := staging.ConfigDir(composeProject, env)
	_ = os.Setenv(constants.EnvConfigDir, filepath.ToSlash(configDir))
	if os.Getenv(constants.EnvProjectName) == "" {
		_ = os.Setenv(constants.EnvProjectName, staging.ComposeProjectKey(composeProject, env))
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
}

func applyModeFromRequest(mode string) error {
	existing, err := envutil.GetHostEnv(constants.HostSuffixMode)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return nil
	}
	return envutil.SetHostEnv(constants.HostSuffixMode, strings.ToLower(mode))
}

// findRepoRoot locates the repository root using shared config logic.
func findRepoRoot(start string) (string, error) {
	return config.ResolveRepoRoot(start)
}
