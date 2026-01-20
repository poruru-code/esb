// Where: cli/internal/helpers/ports.go
// What: Port discovery and persistence helpers.
// Why: Persist compose ports for provisioning and E2E.
package helpers

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// PortDiscoverer defines the interface for discovering dynamically assigned ports
// from running Docker Compose services.
type PortDiscoverer interface {
	Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error)
}

// composePortDiscoverer implements PortDiscoverer using Docker Compose port command.
type composePortDiscoverer struct {
	runner compose.CommandRunner
}

// NewPortDiscoverer creates a PortDiscoverer that uses Docker Compose
// to discover dynamically assigned host ports for services.
func NewPortDiscoverer() PortDiscoverer {
	return composePortDiscoverer{runner: compose.ExecRunner{}}
}

// Discover queries Docker Compose for the host ports of running services
// and returns a map of environment variable names to port numbers.
func (d composePortDiscoverer) Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error) {
	if d.runner == nil {
		return nil, fmt.Errorf("port discovery runner not configured")
	}
	opts := compose.PortDiscoveryOptions{
		RootDir: rootDir,
		Project: project,
		Mode:    mode,
		Target:  "control",
	}
	return compose.DiscoverPorts(ctx, d.runner, opts)
}

// DiscoverAndPersistPorts discovers running service ports and persists them
// to a JSON file for use by provisioning and E2E tests.
func DiscoverAndPersistPorts(
	ctx state.Context,
	discoverer PortDiscoverer,
	store ports.StateStore,
) (ports.PortPublishResult, error) {
	var result ports.PortPublishResult
	if discoverer == nil {
		return result, nil
	}
	if store == nil {
		return result, fmt.Errorf("port state store not configured")
	}
	rootDir, err := config.ResolveRepoRoot(ctx.ProjectDir)
	if err != nil {
		return result, err
	}
	ports, err := discoverer.Discover(context.Background(), rootDir, ctx.ComposeProject, ctx.Mode)
	if err != nil {
		return result, err
	}
	if len(ports) == 0 {
		return result, nil
	}
	previous, err := store.Load(ctx)
	if err != nil {
		return result, err
	}
	if err := store.Save(ctx, ports); err != nil {
		return result, err
	}
	applyPortsToEnv(ports)
	result.Detected = ports
	result.Published = ports
	result.Changed = !portsEqual(previous, ports)
	return result, nil
}

func portsEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if other, ok := right[key]; !ok || other != value {
			return false
		}
	}
	return true
}

// applyPortsToEnv sets environment variables for discovered ports,
// including convenience variables like GATEWAY_URL and VICTORIALOGS_URL.
func applyPortsToEnv(ports map[string]int) {
	for key, value := range ports {
		_ = os.Setenv(key, strconv.Itoa(value))
	}
	if port, ok := ports[constants.EnvPortGatewayHTTPS]; ok {
		_ = os.Setenv(constants.EnvGatewayPort, strconv.Itoa(port))
		_ = os.Setenv(constants.EnvGatewayURL, fmt.Sprintf("https://localhost:%d", port))
	}
	if port, ok := ports[constants.EnvPortVictoriaLogs]; ok {
		_ = os.Setenv(constants.EnvVictoriaLogsPort, strconv.Itoa(port))
		_ = os.Setenv(constants.EnvVictoriaLogsURL, fmt.Sprintf("http://localhost:%d", port))
	}
	if port, ok := ports[constants.EnvPortAgentGRPC]; ok {
		_ = os.Setenv(constants.EnvAgentGrpcAddress, fmt.Sprintf("localhost:%d", port))
	}
}
