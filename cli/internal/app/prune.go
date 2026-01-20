// Where: cli/internal/app/prune.go
// What: Prune command helpers.
// Why: Clean project-scoped Docker resources and generated artifacts safely.
package app

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runPrune executes the 'prune' command which removes project-scoped Docker
// resources and generated artifacts, with a docker system prune-like prompt.
func runPrune(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Prune.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	return runPruneWithDeps(deps.Prune, cli.Prune, ctxInfo, out)
}

func runPruneWithDeps(deps PruneDeps, flags PruneCmd, ctxInfo commandContext, out io.Writer) int {
	if deps.Pruner == nil {
		fmt.Fprintln(out, "prune: pruner not configured")
		return 1
	}

	req := PruneRequest{
		Context:       ctxInfo.Context,
		Hard:          flags.Hard,
		RemoveVolumes: flags.Volumes,
		AllImages:     flags.All,
	}

	printPruneWarning(out, req)
	if !flags.Yes {
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

	workflow := workflows.NewPruneWorkflow(deps.Pruner, ports.NewLegacyUI(out))
	if err := workflow.Run(workflows.PruneRequest(req)); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

func printPruneWarning(out io.Writer, request PruneRequest) {
	fmt.Fprintln(out, "WARNING! This will remove:")
	fmt.Fprintln(out, "  - all stopped project containers")
	fmt.Fprintln(out, "  - all project networks not used by at least one container")
	if request.AllImages {
		fmt.Fprintln(out, "  - all project images not used by at least one container")
	} else {
		fmt.Fprintln(out, "  - all dangling project images")
	}
	if request.RemoveVolumes {
		fmt.Fprintln(out, "  - all project volumes not used by at least one container")
	}
	if request.Context.OutputEnvDir != "" {
		fmt.Fprintf(out, "  - generated artifacts: %s\n", request.Context.OutputEnvDir)
	}
	if request.Hard {
		fmt.Fprintln(out, "  - generator.yml")
	}
	fmt.Fprintln(out, "")
}
