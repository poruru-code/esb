// Where: cli/internal/infra/build/build_env.go
// What: Environment helpers for generator build flows.
// Why: Keep build-related env setup shared without Python builder dependency.
package build

import (
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func applyBuildEnv(env, composeProject string) error {
	if os.Getenv(constants.EnvProjectName) == "" {
		_ = os.Setenv(constants.EnvProjectName, staging.ComposeProjectKey(composeProject, env))
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
	return nil
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
