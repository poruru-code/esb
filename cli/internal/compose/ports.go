// Where: cli/internal/compose/ports.go
// What: Docker compose port discovery helpers.
// Why: Persist dynamic ports for provisioning and tests.
package compose

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CommandOutputer defines the interface for executing commands and capturing output.
type CommandOutputer interface {
	Output(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// PortMapping defines a mapping from container port to environment variable name.
type PortMapping struct {
	EnvVar        string
	Service       string
	ContainerPort int
	Modes         []string
}

// PortDiscoveryOptions contains configuration for discovering ports.
type PortDiscoveryOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	ExtraFiles []string
	Mappings   []PortMapping
}

var DefaultPortMappings = []PortMapping{
	{
		EnvVar:        "ESB_PORT_GATEWAY_HTTPS",
		Service:       "gateway",
		ContainerPort: 443,
		Modes:         []string{ModeDocker},
	},
	{
		EnvVar:        "ESB_PORT_GATEWAY_HTTPS",
		Service:       "runtime-node",
		ContainerPort: 443,
		Modes:         []string{ModeContainerd, ModeFirecracker},
	},
	{EnvVar: "ESB_PORT_S3", Service: "s3-storage", ContainerPort: 9000},
	{EnvVar: "ESB_PORT_S3_MGMT", Service: "s3-storage", ContainerPort: 9001},
	{EnvVar: "ESB_PORT_DATABASE", Service: "database", ContainerPort: 8000},
	{EnvVar: "ESB_PORT_VICTORIALOGS", Service: "victorialogs", ContainerPort: 9428},
	{
		EnvVar:        "ESB_PORT_REGISTRY",
		Service:       "registry",
		ContainerPort: 5010,
		Modes:         []string{ModeContainerd, ModeFirecracker},
	},
	{
		EnvVar:        "ESB_PORT_AGENT_GRPC",
		Service:       "runtime-node",
		ContainerPort: 50051,
		Modes:         []string{ModeContainerd, ModeFirecracker},
	},
}

// Output executes a command and returns its combined stdout/stderr.
func (ExecRunner) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// DiscoverPorts queries docker compose port for each mapping and returns
// the discovered host ports as a map of environment variable names to ports.
func DiscoverPorts(ctx context.Context, runner CommandOutputer, opts PortDiscoveryOptions) (map[string]int, error) {
	if runner == nil {
		return nil, fmt.Errorf("command runner is nil")
	}

	mode := resolveMode(opts.Mode)
	args, err := buildComposeArgs(opts.RootDir, mode, opts.Target, opts.Project, opts.ExtraFiles)
	if err != nil {
		return nil, err
	}

	mappings := opts.Mappings
	if len(mappings) == 0 {
		mappings = DefaultPortMappings
	}

	ports := map[string]int{}
	for _, mapping := range mappings {
		if !modeAllowed(mode, mapping.Modes) {
			continue
		}
		output, err := runner.Output(ctx, opts.RootDir, "docker", append(args, "port", mapping.Service, fmt.Sprintf("%d", mapping.ContainerPort))...)
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(output))
		if raw == "" {
			continue
		}
		idx := strings.LastIndex(raw, ":")
		if idx == -1 || idx+1 >= len(raw) {
			continue
		}
		port, err := strconv.Atoi(raw[idx+1:])
		if err != nil {
			continue
		}
		ports[mapping.EnvVar] = port
	}
	return ports, nil
}

// modeAllowed checks if the current mode is in the allowed list.
// Returns true if allowed list is empty (applies to all modes).
func modeAllowed(mode string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		if value == mode {
			return true
		}
	}
	return false
}
