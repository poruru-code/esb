// Where: cli/internal/app/prune.go
// What: Prune command helpers.
// Why: Remove generated artifacts safely with confirmation.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type PruneRequest struct {
	Context state.Context
	Hard    bool
}

type Pruner interface {
	Prune(request PruneRequest) error
}

func runPrune(cli CLI, deps Dependencies, out io.Writer) int {
	if !cli.Prune.Yes {
		fmt.Fprintln(out, "prune requires confirmation (--yes)")
		return 1
	}
	if deps.Downer == nil {
		fmt.Fprintln(out, "prune: downer not configured")
		return 1
	}

	selection, err := resolveProjectSelection(cli, deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	projectDir := selection.Dir
	if projectDir == "" {
		projectDir = "."
	}

	envDeps := deps
	envDeps.ProjectDir = projectDir
	env := resolveEnv(cli, envDeps)

	fmt.Fprintln(out, "prune warning: containers and volumes will be removed")

	composeProject := fmt.Sprintf("esb-%s", strings.ToLower(env))
	ctx, ctxErr := state.ResolveContext(projectDir, env)
	if ctxErr == nil && ctx.ComposeProject != "" {
		composeProject = ctx.ComposeProject
	}

	if err := deps.Downer.Down(composeProject, true); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if ctxErr != nil {
		fmt.Fprintf(out, "prune skipped artifacts: %v\n", ctxErr)
		fmt.Fprintln(out, "prune complete")
		return 0
	}

	if deps.Pruner == nil {
		fmt.Fprintln(out, "prune: pruner not configured")
		return 1
	}

	if err := deps.Pruner.Prune(PruneRequest{Context: ctx, Hard: cli.Prune.Hard}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "prune complete")
	return 0
}
