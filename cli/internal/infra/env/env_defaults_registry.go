// Where: cli/internal/infra/env/env_defaults_registry.go
// What: Registry default resolution for docker/containerd runtime modes.
// Why: Keep registry host defaults independent from unrelated env setup logic.
package env

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

// applyRegistryDefaults sets registry-related defaults for both docker and containerd modes.
func applyRegistryDefaults(mode string) error {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		normalized = "docker"
	}
	containerRegistry := constants.DefaultContainerRegistry
	if normalized == "docker" {
		containerRegistry = constants.DefaultContainerRegistryHost
	}
	if strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)) == "" {
		_ = os.Setenv(constants.EnvContainerRegistry, containerRegistry)
	}
	if strings.TrimSpace(containerRegistry) != "" {
		registry := containerRegistry
		if !strings.HasSuffix(registry, "/") {
			registry += "/"
		}
		if value, err := envutil.GetHostEnv(constants.HostSuffixRegistry); err == nil {
			if strings.TrimSpace(value) == "" {
				if err := envutil.SetHostEnv(constants.HostSuffixRegistry, registry); err != nil {
					return fmt.Errorf("set host env %s: %w", constants.HostSuffixRegistry, err)
				}
			}
		} else {
			return fmt.Errorf("get host env %s: %w", constants.HostSuffixRegistry, err)
		}
	}
	return nil
}
