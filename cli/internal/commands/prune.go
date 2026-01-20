// Where: cli/internal/commands/prune.go
// What: Prune command helpers.
// Why: Clean project-scoped Docker resources and generated artifacts safely.
package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
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

	cmd, err := newPruneCommand(deps.Prune, out)
	if err != nil {
		return exitWithError(out, err)
	}
	if err := cmd.Run(ctxInfo, cli.Prune); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type pruneCommand struct {
	pruner ports.Pruner
	ui     ports.UserInterface
	out    io.Writer
}

func newPruneCommand(deps PruneDeps, out io.Writer) (*pruneCommand, error) {
	if deps.Pruner == nil {
		return nil, fmt.Errorf("prune: pruner not configured")
	}
	return &pruneCommand{
		pruner: deps.Pruner,
		ui:     ports.NewLegacyUI(out),
		out:    out,
	}, nil
}

func (c *pruneCommand) Run(ctxInfo commandContext, flags PruneCmd) error {
	req := ports.PruneRequest{
		Context:       ctxInfo.Context,
		Hard:          flags.Hard,
		RemoveVolumes: flags.Volumes,
		AllImages:     flags.All,
	}

	printPruneWarning(c.out, req)
	if !flags.Yes {
		if !interaction.IsTerminal(os.Stdin) {
			return fmt.Errorf("prune requires --yes in non-interactive mode")
		}
		confirmed, err := interaction.PromptYesNo("Are you sure you want to continue?")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(c.out, "Aborted.")
			return fmt.Errorf("aborted")
		}
	}

	if err := workflows.NewPruneWorkflow(c.pruner, c.ui).Run(workflows.PruneRequest{
		Context:       req.Context,
		Hard:          req.Hard,
		RemoveVolumes: req.RemoveVolumes,
		AllImages:     req.AllImages,
	}); err != nil {
		return err
	}
	return nil
}

func printPruneWarning(out io.Writer, request ports.PruneRequest) {
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
