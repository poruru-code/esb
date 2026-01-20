// Where: cli/internal/app/stop.go
// What: Stop command helpers.
// Why: Stop environments without removing containers.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runStop executes the 'stop' command which stops all containers
// but preserves their state for later restart with 'up'.
func runStop(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Stop.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return runStopWithDeps(deps.Stop, deps.RepoResolver, ctxInfo, out)
}

func runStopWithDeps(deps StopDeps, repoResolver func(string) (string, error), ctxInfo commandContext, out io.Writer) int {
	if deps.Stopper == nil {
		fmt.Fprintln(out, "stop: not implemented")
		return 1
	}

	workflow := workflows.NewStopWorkflow(
		deps.Stopper,
		newRuntimeEnvApplier(repoResolver),
		ports.NewLegacyUI(out),
	)
	if err := workflow.Run(workflows.StopRequest{Context: ctxInfo.Context}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}
