// Where: cli/internal/infra/deploy/compose_provisioner_status.go
// What: Service running status checks for deploy warnings.
// Why: Keep status probing separate from provisioner run flow.
package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

// CheckServicesStatus checks if gateway and agent/runtime-node are running (warning only).
func (p composeProvisioner) CheckServicesStatus(composeProject, mode string) {
	if p.userInterface == nil {
		return
	}

	gatewayRunning := p.isServiceRunning(composeProject, "gateway")
	if !gatewayRunning {
		p.userInterface.Warn("Warning: Gateway is not running. Deploy will continue but functions may not be immediately available.")
	}

	agentService := "agent"
	if mode == compose.ModeContainerd {
		agentService = "runtime-node"
	}
	agentRunning := p.isServiceRunning(composeProject, agentService)
	if !agentRunning {
		p.userInterface.Warn(
			fmt.Sprintf(
				"Warning: %s is not running. Deploy will continue but function execution may fail.",
				agentService,
			),
		)
	}
}

func (p composeProvisioner) isServiceRunning(composeProject, service string) bool {
	if p.composeRunner == nil {
		return true
	}
	ctx := context.Background()
	args := []string{"compose"}
	if result, err := p.resolveComposeFilesForProject(ctx, composeProject); err == nil {
		for _, file := range result.Files {
			args = append(args, "-f", file)
		}
	}
	if strings.TrimSpace(composeProject) != "" {
		args = append(args, "-p", composeProject)
	}
	out, err := p.composeRunner.RunOutput(ctx, "", "docker", append(args, "ps", "-q", service)...)
	if err != nil {
		return false
	}
	return len(out) > 0
}
