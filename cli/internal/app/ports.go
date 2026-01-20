// Where: cli/internal/app/ports.go
// What: Port discovery and persistence helpers.
// Why: Persist compose ports for provisioning and E2E.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/meta"
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
func DiscoverAndPersistPorts(ctx state.Context, discoverer PortDiscoverer) (map[string]int, error) {
	if discoverer == nil {
		return nil, nil
	}
	rootDir, err := config.ResolveRepoRoot(ctx.ProjectDir)
	if err != nil {
		return nil, err
	}
	ports, err := discoverer.Discover(context.Background(), rootDir, ctx.ComposeProject, ctx.Mode)
	if err != nil {
		return nil, err
	}
	if len(ports) == 0 {
		return nil, nil
	}
	if _, err := savePorts(ctx.Env, ports); err != nil {
		return nil, err
	}
	applyPortsToEnv(ports)
	return ports, nil
}

func PrintDiscoveredPorts(ui ports.UserInterface, discovered map[string]int) {
	if len(discovered) == 0 || ui == nil {
		return
	}

	var rows []ports.KeyValue
	addPort := func(key string) {
		if val, ok := discovered[key]; ok {
			rows = append(rows, ports.KeyValue{Key: key, Value: val})
		}
	}

	addPort(constants.EnvPortGatewayHTTPS)
	addPort(constants.EnvPortVictoriaLogs)
	addPort(constants.EnvPortDatabase)
	addPort(constants.EnvPortS3)
	addPort(constants.EnvPortS3Mgmt)
	addPort(constants.EnvPortRegistry)

	var unknownKeys []string
	for key := range discovered {
		if !isKnownPort(key) {
			unknownKeys = append(unknownKeys, key)
		}
	}
	sort.Strings(unknownKeys)
	for _, key := range unknownKeys {
		rows = append(rows, ports.KeyValue{Key: key, Value: discovered[key]})
	}

	if len(rows) == 0 {
		return
	}

	ui.Block("🔌", "Discovered Ports:", rows)
}

func isKnownPort(key string) bool {
	switch key {
	case constants.EnvPortGatewayHTTPS, constants.EnvPortVictoriaLogs, constants.EnvPortDatabase, constants.EnvPortS3, constants.EnvPortS3Mgmt, constants.EnvPortRegistry, constants.EnvPortAgentGRPC:
		return true
	}
	return false
}

// savePorts writes the discovered ports to a JSON file in the project-specific data directory.
// Returns the path to the saved file.
func savePorts(env string, ports map[string]int) (string, error) {
	home, err := resolveESBHome(env)
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, "ports.json")
	payload, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// resolveESBHome determines the base directory for project-specific data.
// Uses brand-specific HOME environment variable if set, otherwise ~/.<brand>/<env>.
func resolveESBHome(env string) (string, error) {
	override := strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixHome))
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(env)
	if name == "" {
		name = "default"
	}
	homeDirName := meta.HomeDir
	if !strings.HasPrefix(homeDirName, ".") {
		homeDirName = "." + homeDirName
	}
	return filepath.Join(home, homeDirName, name), nil
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
