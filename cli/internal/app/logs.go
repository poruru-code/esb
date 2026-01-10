// Where: cli/internal/app/logs.go
// What: Logs command helpers.
// Why: Provide log access via docker compose with CLI flags.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// LogsRequest contains parameters for viewing container logs.
// It specifies follow mode, tail count, timestamps, and optional service filter.
type LogsRequest struct {
	Context    state.Context
	Follow     bool
	Tail       int
	Timestamps bool
	Service    string
}

// Logger defines the interface for streaming container logs.
// Implementations use Docker Compose to retrieve log output.
type Logger interface {
	Logs(request LogsRequest) error
}

// runLogs executes the 'logs' command which streams container logs
// with optional follow, tail, and timestamp options.
func runLogs(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Logger == nil {
		fmt.Fprintln(out, "logs: not implemented")
		return 1
	}

	opts := newResolveOptions(cli.Logs.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	ctx := ctxInfo.Context
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
