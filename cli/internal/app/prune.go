// Where: cli/internal/app/prune.go
// What: Prune command helpers.
// Why: Clean ESB Docker resources and generated artifacts safely.
package app

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// PruneRequest contains parameters for removing ESB resources and artifacts.
// The Hard flag also removes the generator.yml configuration file.
type PruneRequest struct {
	Context       state.Context
	Hard          bool
	RemoveVolumes bool
	AllImages     bool
}

// Pruner defines the interface for removing ESB resources and artifacts.
// Implementations prune Docker resources and optionally clean configuration.
type Pruner interface {
	Prune(request PruneRequest) error
}

// runPrune executes the 'prune' command which removes ESB-scoped Docker
// resources and generated artifacts, with a docker system prune-like prompt.
func runPrune(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Prune.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	if deps.Pruner == nil {
		fmt.Fprintln(out, "prune: pruner not configured")
		return 1
	}

	req := PruneRequest{
		Context:       ctxInfo.Context,
		Hard:          cli.Prune.Hard,
		RemoveVolumes: cli.Prune.Volumes,
		AllImages:     cli.Prune.All,
	}

	printPruneWarning(out, req)
	if !cli.Prune.Yes {
		if !isTerminal(os.Stdin) {
			return exitWithError(out, fmt.Errorf("prune requires --yes in non-interactive mode"))
		}
		confirmed, err := promptYesNo("Are you sure you want to continue?")
		if err != nil {
			return exitWithError(out, err)
		}
		if !confirmed {
			fmt.Fprintln(out, "Aborted.")
			return 1
		}
	}

	if err := deps.Pruner.Prune(req); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintln(out, "prune complete")
	return 0
}

func printPruneWarning(out io.Writer, request PruneRequest) {
	fmt.Fprintln(out, "WARNING! This will remove:")
	fmt.Fprintln(out, "  - all stopped ESB containers")
	fmt.Fprintln(out, "  - all ESB networks not used by at least one container")
	if request.AllImages {
		fmt.Fprintln(out, "  - all ESB images not used by at least one container")
	} else {
		fmt.Fprintln(out, "  - all dangling ESB images")
	}
	if request.RemoveVolumes {
		fmt.Fprintln(out, "  - all ESB volumes not used by at least one container")
	}
	if request.Context.OutputEnvDir != "" {
		fmt.Fprintf(out, "  - ESB generated artifacts: %s\n", request.Context.OutputEnvDir)
	}
	if request.Hard {
		fmt.Fprintln(out, "  - generator.yml")
	}
	fmt.Fprintln(out, "")
}
