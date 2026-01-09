// Where: tools-go/internal/app/down.go
// What: Down command helpers.
// Why: Stop and remove resources for an environment.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/tools-go/internal/state"
)

type Downer interface {
	Down(project string, removeVolumes bool) error
}

func runDown(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Downer == nil {
		fmt.Fprintln(out, "down: not implemented")
		return 1
	}

	projectDir := deps.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	env := resolveEnv(cli, deps)

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if err := deps.Downer.Down(ctx.ComposeProject, cli.Down.Volumes); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "down complete")
	return 0
}
