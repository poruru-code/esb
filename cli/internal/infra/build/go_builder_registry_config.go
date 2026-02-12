// Where: cli/internal/infra/build/go_builder_registry_config.go
// What: Registry resolution helpers used by GoBuilder orchestration.
// Why: Separate registry address decisions from Build flow control.
package build

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

type registryConfig struct {
	Registry string
}

type buildRegistryInfo struct {
	ServiceRegistry    string
	RuntimeRegistry    string
	PushRegistry       string
	BuilderNetworkMode string
}

func resolveRegistryConfig() (registryConfig, error) {
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		return registryConfig{}, err
	}
	registry := strings.TrimSpace(os.Getenv(key))
	if registry == "" {
		registry = constants.DefaultContainerRegistry
	}
	if !strings.HasSuffix(registry, "/") {
		registry += "/"
	}
	return registryConfig{Registry: registry}, nil
}

func (b *GoBuilder) resolveBuildRegistryInfo(
	ctx context.Context,
	repoRoot string,
	composeProject string,
	request BuildRequest,
) (buildRegistryInfo, error) {
	registry, err := resolveRegistryConfig()
	if err != nil {
		return buildRegistryInfo{}, err
	}

	runtimeRegistry := resolveRuntimeRegistry(registry.Registry)
	registryForPush := registry.Registry
	builderNetworkMode := ""

	if registryForPush != "" {
		registryHost := resolveRegistryHost(registryForPush)
		if isLocalRegistryHost(registryHost) {
			hostRegistryAddr, explicitHostAddr := resolveHostRegistryAddress()
			if strings.EqualFold(registryHost, "registry") {
				// Buildx needs host networking for external pulls; push via host-mapped registry port.
				builderNetworkMode = "host"
				registryForPush = fmt.Sprintf("%s/", hostRegistryAddr)
			} else {
				builderNetworkMode = "host"
			}

			if b.PortDiscoverer != nil && !explicitHostAddr {
				ports, err := b.PortDiscoverer.Discover(ctx, repoRoot, composeProject, request.Mode)
				if err != nil {
					return buildRegistryInfo{}, err
				}
				if port, ok := ports[constants.EnvPortRegistry]; ok && port > 0 {
					hostRegistryAddr = fmt.Sprintf("127.0.0.1:%d", port)
					if strings.EqualFold(registryHost, "registry") {
						registryForPush = fmt.Sprintf("127.0.0.1:%d/", port)
					}
					if strings.EqualFold(registryHost, "localhost") || registryHost == "127.0.0.1" {
						registryForPush = fmt.Sprintf("127.0.0.1:%d/", port)
					}
				}
			}

			if err := waitForRegistry(hostRegistryAddr, 30*time.Second); err != nil {
				return buildRegistryInfo{}, err
			}
		}
	}

	return buildRegistryInfo{
		ServiceRegistry:    registry.Registry,
		RuntimeRegistry:    runtimeRegistry,
		PushRegistry:       registryForPush,
		BuilderNetworkMode: builderNetworkMode,
	}, nil
}
