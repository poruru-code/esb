// Where: cli/internal/app/down.go
// What: Down command helpers.
// Why: Stop and remove resources for an environment.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runDown executes the 'down' command which stops all containers
// and optionally removes volumes for the current environment.
func runDown(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Down.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	cmd, err := newDownCommand(deps.Down, out)
	if err != nil {
		return exitWithError(out, err)
	}
	if err := cmd.Run(ctxInfo, cli.Down.Volumes); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type downCommand struct {
	downer ports.Downer
	ui     ports.UserInterface
}

func newDownCommand(deps DownDeps, out io.Writer) (*downCommand, error) {
	if deps.Downer == nil {
		return nil, fmt.Errorf("down: not implemented")
	}
	return &downCommand{
		downer: deps.Downer,
		ui:     ports.NewLegacyUI(out),
	}, nil
}

func (c *downCommand) Run(ctxInfo commandContext, volumes bool) error {
	return workflows.NewDownWorkflow(c.downer, c.ui).Run(workflows.DownRequest{
		Context: ctxInfo.Context,
		Volumes: volumes,
	})
}
