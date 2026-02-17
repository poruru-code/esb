// Where: cli/internal/infra/env/env_defaults.go
// What: Runtime environment default entrypoint for deploy/build execution.
// Why: Keep the setup sequence explicit while delegating details by responsibility.
package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

var errEnvPrefixRequired = errors.New("ENV_PREFIX is required")

// ApplyRuntimeEnv sets all environment variables required for running commands,
// including project metadata, ports, networks, and custom generator parameters.
func ApplyRuntimeEnv(ctx state.Context, resolver func(string) (string, error)) error {
	projectRoot := strings.TrimSpace(ctx.ProjectDir)
	if projectRoot == "" {
		startDir := strings.TrimSpace(ctx.TemplatePath)
		if startDir != "" {
			startDir = filepath.Dir(startDir)
		} else if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		}
		if resolver == nil {
			return fmt.Errorf("project root is required to apply runtime env")
		}
		resolved, err := resolver(startDir)
		if err != nil {
			return fmt.Errorf("resolve repo root: %w", err)
		}
		projectRoot = resolved
	}
	if err := ApplyBrandingEnvWithRoot(projectRoot); err != nil {
		return err
	}
	if err := applyModeEnv(ctx.Mode); err != nil {
		return err
	}

	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		env = "default"
	}

	// Host-level variables (with prefix) for brand-aware logic.
	if err := envutil.SetHostEnv(constants.HostSuffixEnv, env); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixEnv, err)
	}
	if err := envutil.SetHostEnv(constants.HostSuffixMode, ctx.Mode); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixMode, err)
	}

	// Compose variables (no prefix) for docker-compose.yml reference.
	setEnvIfEmpty("ENV", env)
	setEnvIfEmpty("MODE", ctx.Mode)

	// Compose project metadata.
	if os.Getenv(constants.EnvProjectName) == "" {
		_ = os.Setenv(constants.EnvProjectName, ctx.ComposeProject)
	}

	applyPortDefaults(env)
	applySubnetDefaults(env)
	if err := applyRegistryDefaults(ctx.Mode); err != nil {
		return err
	}
	if err := applyConfigDirEnv(ctx, resolver); err != nil {
		return err
	}
	if err := applyProxyDefaults(); err != nil {
		return err
	}
	if err := normalizeRegistryEnv(); err != nil {
		return err
	}

	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
	setEnvIfEmpty("BUILDX_BUILDER", fmt.Sprintf("%s-buildx", meta.Slug))
	return nil
}
