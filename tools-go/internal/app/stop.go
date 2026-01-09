// Where: tools-go/internal/app/stop.go
// What: Stop command helpers.
// Why: Stop environments without removing containers.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/tools-go/internal/state"
)

type StopRequest struct {
	Context state.Context
}

type Stopper interface {
	Stop(request StopRequest) error
}

func runStop(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Stopper == nil {
		fmt.Fprintln(out, "stop: not implemented")
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

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	applyModeEnv(ctx.Mode)
	applyEnvironmentDefaults(ctx.Env, ctx.Mode)

	if err := deps.Stopper.Stop(StopRequest{Context: ctx}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "stop complete")
	return 0
}
