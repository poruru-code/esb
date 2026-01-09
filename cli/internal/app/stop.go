// Where: cli/internal/app/stop.go
// What: Stop command helpers.
// Why: Stop environments without removing containers.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
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

	ctxInfo, err := resolveCommandContext(cli, deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	ctx := ctxInfo.Context
	applyModeEnv(ctx.Mode)
	applyEnvironmentDefaults(ctx.Env, ctx.Mode)

	if err := deps.Stopper.Stop(StopRequest{Context: ctx}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "stop complete")
	return 0
}
