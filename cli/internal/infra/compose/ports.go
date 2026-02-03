// Where: cli/internal/infra/compose/ports.go
// What: Docker compose port discovery env.
// Why: Persist dynamic ports for provisioning and tests.
package compose

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
)

// PortMapping defines a mapping from container port to environment variable name.
type PortMapping struct {
	EnvVar        string
	Service       string
	ContainerPort int
	Modes         []string
}

// PortDiscoveryOptions contains configuration for discovering ports.
type PortDiscoveryOptions struct {
	RootDir      string
	Project      string
	Mode         string
	Target       string
	ExtraFiles   []string
	ComposeFiles []string
	Mappings     []PortMapping
}

var DefaultPortMappings = []PortMapping{
	{
		EnvVar:        constants.EnvPortGatewayHTTPS,
		Service:       "gateway",
		ContainerPort: 8443,
		Modes:         []string{ModeDocker},
	},
	{
		EnvVar:        constants.EnvPortGatewayHTTPS,
		Service:       "runtime-node",
		ContainerPort: 8443,
		Modes:         []string{ModeContainerd},
	},
	{EnvVar: constants.EnvPortS3, Service: "s3-storage", ContainerPort: 9000},
	{EnvVar: constants.EnvPortS3Mgmt, Service: "s3-storage", ContainerPort: 9001},
	{EnvVar: constants.EnvPortDatabase, Service: "database", ContainerPort: 8000},
	{EnvVar: constants.EnvPortVictoriaLogs, Service: "victorialogs", ContainerPort: 9428},
	{
		EnvVar:        constants.EnvPortRegistry,
		Service:       "registry",
		ContainerPort: 5010,
		Modes:         []string{ModeContainerd, ModeDocker},
	},
	{
		EnvVar:        constants.EnvPortAgentGRPC,
		Service:       "runtime-node",
		ContainerPort: 50051,
		Modes:         []string{ModeContainerd},
	},
	{
		EnvVar:        constants.EnvPortAgentMetrics,
		Service:       "agent",
		ContainerPort: 9091,
		Modes:         []string{ModeDocker},
	},
	{
		EnvVar:        constants.EnvPortAgentMetrics,
		Service:       "runtime-node",
		ContainerPort: 9091,
		Modes:         []string{ModeContainerd},
	},
}

// ExecRunner is implemented in up.go.

// DiscoverPorts queries docker compose port for each mapping and returns
// the discovered host ports as a map of environment variable names to ports.
func DiscoverPorts(ctx context.Context, runner CommandRunner, opts PortDiscoveryOptions) (map[string]int, error) {
	if runner == nil {
		return nil, errCommandRunnerNil
	}

	mode := resolveMode(opts.Mode)
	var args []string
	if len(opts.ComposeFiles) > 0 {
		args = []string{"compose"}
		if strings.TrimSpace(opts.Project) != "" {
			args = append(args, "-p", opts.Project)
		}
		seen := map[string]struct{}{}
		for _, file := range opts.ComposeFiles {
			trimmed := strings.TrimSpace(file)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			args = append(args, "-f", trimmed)
			seen[trimmed] = struct{}{}
		}
		for _, file := range opts.ExtraFiles {
			trimmed := strings.TrimSpace(file)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			args = append(args, "-f", trimmed)
			seen[trimmed] = struct{}{}
		}
	} else {
		var err error
		args, err = buildComposeArgs(opts.RootDir, mode, opts.Target, opts.Project, opts.ExtraFiles)
		if err != nil {
			return nil, err
		}
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
		output, err := runner.RunOutput(ctx, opts.RootDir, "docker", append(args, "port", mapping.Service, fmt.Sprintf("%d", mapping.ContainerPort))...)
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
		if err != nil || port == 0 {
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
