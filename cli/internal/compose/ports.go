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

type CommandOutputer interface {
	Output(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type PortMapping struct {
	EnvVar        string
	Service       string
	ContainerPort int
	Modes         []string
}

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
	{EnvVar: "ESB_PORT_STORAGE", Service: "s3-storage", ContainerPort: 9000},
	{EnvVar: "ESB_PORT_STORAGE_MGMT", Service: "s3-storage", ContainerPort: 9001},
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

func (ExecRunner) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func DiscoverPorts(ctx context.Context, runner CommandOutputer, opts PortDiscoveryOptions) (map[string]int, error) {
	if runner == nil {
		return nil, fmt.Errorf("command runner is nil")
	}
	if opts.RootDir == "" {
		return nil, fmt.Errorf("root dir is required")
	}

	mode := resolveMode(opts.Mode)
	files, err := ResolveComposeFiles(opts.RootDir, mode, opts.Target)
	if err != nil {
		return nil, err
	}
	args := []string{"compose"}
	if opts.Project != "" {
		args = append(args, "-p", opts.Project)
	}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	for _, file := range opts.ExtraFiles {
		if strings.TrimSpace(file) == "" {
			continue
		}
		args = append(args, "-f", file)
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
