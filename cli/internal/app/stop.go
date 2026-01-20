// Where: cli/internal/app/stop.go
// What: Stop command helpers.
// Why: Stop environments without removing containers.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runStop executes the 'stop' command which stops all containers
// but preserves their state for later restart with 'up'.
func runStop(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Stop.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	cmd, err := newStopCommand(deps.Stop, repoResolver, out)
	if err != nil {
		return exitWithError(out, err)
	}
	if err := cmd.Run(ctxInfo); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type stopCommand struct {
	stopper    ports.Stopper
	envApplier ports.RuntimeEnvApplier
	ui         ports.UserInterface
}

func newStopCommand(deps StopDeps, repoResolver func(string) (string, error), out io.Writer) (*stopCommand, error) {
	if deps.Stopper == nil {
		return nil, fmt.Errorf("stop: not implemented")
	}
	return &stopCommand{
		stopper:    deps.Stopper,
		envApplier: newRuntimeEnvApplier(repoResolver),
		ui:         ports.NewLegacyUI(out),
	}, nil
}

func (c *stopCommand) Run(ctxInfo commandContext) error {
	if c.envApplier != nil {
		c.envApplier.Apply(ctxInfo.Context)
	}
	return workflows.NewStopWorkflow(c.stopper, c.envApplier, c.ui).Run(workflows.StopRequest{Context: ctxInfo.Context})
}
