// Where: cli/internal/app/down.go
// What: Down command helpers.
// Why: Stop and remove resources for an environment.
package app

import (
	"fmt"
	"io"
)

type Downer interface {
	Down(project string, removeVolumes bool) error
}

func runDown(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Downer == nil {
		fmt.Fprintln(out, "down: not implemented")
		return 1
	}

	ctxInfo, err := resolveCommandContext(cli, deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if err := deps.Downer.Down(ctxInfo.Context.ComposeProject, cli.Down.Volumes); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "down complete")
	return 0
}
