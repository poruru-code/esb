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
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type PortDiscoverer interface {
	Discover(ctx state.Context) (map[string]int, error)
}

type composePortDiscoverer struct {
	runner compose.CommandOutputer
}

func NewPortDiscoverer() PortDiscoverer {
	return composePortDiscoverer{runner: compose.ExecRunner{}}
}

func (d composePortDiscoverer) Discover(ctx state.Context) (map[string]int, error) {
	if d.runner == nil {
		return nil, fmt.Errorf("port discovery runner not configured")
	}
	rootDir, err := compose.FindRepoRoot(ctx.ProjectDir)
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

func discoverAndPersistPorts(ctx state.Context, discoverer PortDiscoverer, out io.Writer) {
	if discoverer == nil {
		return
	}
	ports, err := discoverer.Discover(ctx)
	if err != nil {
		fmt.Fprintln(out, err)
		return
	}
	if len(ports) == 0 {
		return
	}
	if _, err := savePorts(ctx.Env, ports); err != nil {
		fmt.Fprintln(out, err)
		return
	}
	applyPortsToEnv(ports)
}

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

func resolveESBHome(env string) (string, error) {
	override := strings.TrimSpace(os.Getenv("ESB_HOME"))
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

func applyPortsToEnv(ports map[string]int) {
	for key, value := range ports {
		_ = os.Setenv(key, strconv.Itoa(value))
	}
	if port, ok := ports["ESB_PORT_GATEWAY_HTTPS"]; ok {
		_ = os.Setenv("GATEWAY_PORT", strconv.Itoa(port))
		_ = os.Setenv("GATEWAY_URL", fmt.Sprintf("https://localhost:%d", port))
	}
	if port, ok := ports["ESB_PORT_VICTORIALOGS"]; ok {
		_ = os.Setenv("VICTORIALOGS_PORT", strconv.Itoa(port))
		_ = os.Setenv("VICTORIALOGS_URL", fmt.Sprintf("http://localhost:%d", port))
		_ = os.Setenv("VICTORIALOGS_QUERY_URL", fmt.Sprintf("http://localhost:%d", port))
	}
	if port, ok := ports["ESB_PORT_AGENT_GRPC"]; ok {
		_ = os.Setenv("AGENT_GRPC_ADDRESS", fmt.Sprintf("localhost:%d", port))
	}
}
