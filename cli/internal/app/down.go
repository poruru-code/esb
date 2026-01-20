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
		fmt.Fprintln(out, err)
		return 1
	}
	return runDownWithDeps(deps.Down, ctxInfo, cli.Down.Volumes, out)
}

func runDownWithDeps(deps DownDeps, ctxInfo commandContext, volumes bool, out io.Writer) int {
	if deps.Downer == nil {
		fmt.Fprintln(out, "down: not implemented")
		return 1
	}

	workflow := workflows.NewDownWorkflow(deps.Downer, ports.NewLegacyUI(out))
	if err := workflow.Run(workflows.DownRequest{
		Context: ctxInfo.Context,
		Volumes: volumes,
	}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}
