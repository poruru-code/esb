// Where: cli/internal/app/ports.go
// What: Port discovery and persistence helpers.
// Why: Persist compose ports for provisioning and E2E.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/ui"
)

// PortDiscoverer defines the interface for discovering dynamically assigned ports
// from running Docker Compose services.
type PortDiscoverer interface {
	Discover(ctx state.Context) (map[string]int, error)
}

// composePortDiscoverer implements PortDiscoverer using Docker Compose port command.
type composePortDiscoverer struct {
	runner compose.CommandOutputer
}

// NewPortDiscoverer creates a PortDiscoverer that uses Docker Compose
// to discover dynamically assigned host ports for services.
func NewPortDiscoverer() PortDiscoverer {
	return composePortDiscoverer{runner: compose.ExecRunner{}}
}

// Discover queries Docker Compose for the host ports of running services
// and returns a map of environment variable names to port numbers.
func (d composePortDiscoverer) Discover(ctx state.Context) (map[string]int, error) {
	if d.runner == nil {
		return nil, fmt.Errorf("port discovery runner not configured")
	}
	rootDir, err := config.ResolveRepoRoot(ctx.ProjectDir)
	if err != nil {
		return nil, err
	}
	opts := compose.PortDiscoveryOptions{
		RootDir: rootDir,
		Project: ctx.ComposeProject,
		Mode:    ctx.Mode,
		Target:  "control",
	}
	return compose.DiscoverPorts(context.Background(), d.runner, opts)
}

// DiscoverAndPersistPorts discovers running service ports and persists them
// to a JSON file for use by provisioning and E2E tests.
func DiscoverAndPersistPorts(ctx state.Context, discoverer PortDiscoverer, out io.Writer) map[string]int {
	if discoverer == nil {
		return nil
	}
	ports, err := discoverer.Discover(ctx)
	if err != nil {
		fmt.Fprintln(out, err)
		return nil
	}
	if len(ports) == 0 {
		return nil
	}
	if _, err := savePorts(ctx.Env, ports); err != nil {
		fmt.Fprintln(out, err)
		return nil
	}
	applyPortsToEnv(ports)
	return ports
}

func PrintDiscoveredPorts(out io.Writer, ports map[string]int) {
	if len(ports) == 0 {
		return
	}
	console := ui.New(out)
	console.Header("🔌", "Discovered Ports:")

	// Print known critical ports first for better UX
	printPort(console, ports, constants.EnvPortGatewayHTTPS, "Gateway HTTPS")
	printPort(console, ports, constants.EnvPortVictoriaLogs, "VictoriaLogs")
	printPort(console, ports, constants.EnvPortDatabase, "ScyllaDB")
	printPort(console, ports, constants.EnvPortS3, "RustFS")
	printPort(console, ports, constants.EnvPortS3Mgmt, "S3 Management")
	printPort(console, ports, constants.EnvPortRegistry, "Registry")

	// Print remaining unknown ports
	for k, v := range ports {
		if !isKnownPort(k) {
			console.Item(k, v)
		}
	}
}

func isKnownPort(key string) bool {
	switch key {
	case constants.EnvPortGatewayHTTPS, constants.EnvPortVictoriaLogs, constants.EnvPortDatabase, constants.EnvPortS3, constants.EnvPortS3Mgmt, constants.EnvPortRegistry, constants.EnvPortAgentCGRPC:
		return true
	}
	return false
}

func printPort(console *ui.Console, ports map[string]int, key, label string) {
	if val, ok := ports[key]; ok {
		// Use manual formatting to include the key name in parenthesis if needed,
		// or extend Console.Item to support it. For now, match the detailed "Key (Env): Val" style.
		// Or simplified: use Item with formatted key.
		displayKey := fmt.Sprintf("%s (%s)", label, key)
		console.Item(displayKey, val)
	}
}

// savePorts writes the discovered ports to a JSON file in the ESB home directory.
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

// resolveESBHome returns the ESB home directory for the given environment.
// Uses ESB_HOME environment variable if set, otherwise ~/.esb/<env>.
func resolveESBHome(env string) (string, error) {
	override := strings.TrimSpace(os.Getenv(constants.EnvESBHome))
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
	return filepath.Join(home, ".esb", name), nil
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
		_ = os.Setenv(constants.EnvVictoriaLogsQueryURL, fmt.Sprintf("http://localhost:%d", port))
	}
	if port, ok := ports[constants.EnvPortAgentCGRPC]; ok {
		_ = os.Setenv(constants.EnvAgentGrpcAddress, fmt.Sprintf("localhost:%d", port))
	}
}
