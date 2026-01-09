// Where: tools-go/internal/app/logs.go
// What: Logs command helpers.
// Why: Provide log access via docker compose with CLI flags.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/tools-go/internal/state"
)

type LogsRequest struct {
	Context    state.Context
	Follow     bool
	Tail       int
	Timestamps bool
	Service    string
}

type Logger interface {
	Logs(request LogsRequest) error
}

func runLogs(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Logger == nil {
		fmt.Fprintln(out, "logs: not implemented")
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

	req := LogsRequest{
		Context:    ctx,
		Follow:     cli.Logs.Follow,
		Tail:       cli.Logs.Tail,
		Timestamps: cli.Logs.Timestamps,
		Service:    strings.TrimSpace(cli.Logs.Service),
	}
	if err := deps.Logger.Logs(req); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}
