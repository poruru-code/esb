// Where: cli/internal/app/prune.go
// What: Prune command helpers.
// Why: Remove generated artifacts safely with confirmation.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// PruneRequest contains parameters for removing generated artifacts.
// The Hard flag also removes the generator.yml configuration file.
type PruneRequest struct {
	Context state.Context
	Hard    bool
}

// Pruner defines the interface for removing generated artifacts.
// Implementations clean up the output directory and optionally configuration.
type Pruner interface {
	Prune(request PruneRequest) error
}

// runPrune executes the 'prune' command which stops containers,
// removes volumes, and deletes generated artifacts from the output directory.
func runPrune(cli CLI, deps Dependencies, out io.Writer) int {
	if !cli.Prune.Yes {
		fmt.Fprintln(out, "prune requires confirmation (--yes)")
		return 1
	}
	if deps.Downer == nil {
		fmt.Fprintln(out, "prune: downer not configured")
		return 1
	}

	opts := newResolveOptions(cli.Prune.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintln(out, "prune warning: containers and volumes will be removed")

	ctx := ctxInfo.Context
	if err := deps.Downer.Down(ctx.ComposeProject, true); err != nil {
		return exitWithError(out, err)
	}

	if deps.Pruner == nil {
		fmt.Fprintln(out, "prune: pruner not configured")
		return 1
	}

	if err := deps.Pruner.Prune(PruneRequest{Context: ctx, Hard: cli.Prune.Hard}); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintln(out, "prune complete")
	return 0
}
