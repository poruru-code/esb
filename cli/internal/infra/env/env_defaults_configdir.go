// Where: cli/internal/infra/env/env_defaults_configdir.go
// What: CONFIG_DIR and helper environment setters used during runtime setup.
// Why: Isolate staging path detection and low-level env writes from orchestration.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

// applyConfigDirEnv sets the CONFIG_DIR environment variable
// based on the discovered project structure.
func applyConfigDirEnv(ctx state.Context, resolver func(string) (string, error)) error {
	_ = resolver

	stagingAbs, err := staging.ConfigDir(ctx.TemplatePath, ctx.ComposeProject, ctx.Env)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	val := filepath.ToSlash(stagingAbs)
	if err := envutil.SetHostEnv(constants.HostSuffixConfigDir, val); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixConfigDir, err)
	}
	if err := os.Setenv(constants.EnvConfigDir, val); err != nil {
		return fmt.Errorf("set %s: %w", constants.EnvConfigDir, err)
	}
	return nil
}

// setEnvIfEmpty sets an environment variable only if it's currently empty.
func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}

func setHostEnvIfEmpty(suffix, value string) error {
	key, err := envutil.HostEnvKey(suffix)
	if err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return nil
	}
	return envutil.SetHostEnv(suffix, value)
}
