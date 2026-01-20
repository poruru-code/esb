// Where: cli/internal/app/logs.go
// What: Logs command helpers.
// Why: Provide log access via docker compose with CLI flags.
package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// LogsRequest contains parameters for viewing container logs.
// It specifies follow mode, tail count, timestamps, and optional service filter.
// runLogs executes the 'logs' command which streams container logs
// with optional follow, tail, and timestamp options.
func runLogs(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Logs.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return runLogsWithDeps(cli, deps.Logs, deps.Prompter, deps.RepoResolver, ctxInfo, out)
}

func runLogsWithDeps(cli CLI, deps LogsDeps, prompter Prompter, repoResolver func(string) (string, error), ctxInfo commandContext, out io.Writer) int {
	if deps.Logger == nil {
		fmt.Fprintln(out, "logs: not implemented")
		return 1
	}

	ctx := ctxInfo.Context
	req := workflows.LogsRequest{
		LogsRequest: ports.LogsRequest{
			Context:    ctx,
			Follow:     cli.Logs.Follow,
			Tail:       cli.Logs.Tail,
			Timestamps: cli.Logs.Timestamps,
			Service:    strings.TrimSpace(cli.Logs.Service),
		},
	}

	if req.Service == "" && isTerminal(os.Stdin) {
		services, err := deps.Logger.ListServices(req.LogsRequest)
		if err != nil {
			// If listing services fails (e.g. interpolation error), logs will likely fail too.
			// Exit early to avoid double error output.
			return exitWithError(out, err)
		}
		if len(services) > 0 {
			var options []selectOption
			options = append(options, selectOption{Label: "All services", Value: ""})
			for _, svc := range services {
				options = append(options, selectOption{Label: svc, Value: svc})
			}

			if prompter == nil {
				return exitWithError(out, fmt.Errorf("prompter not configured"))
			}
			selected, err := prompter.SelectValue("Select service to view logs", options)
			if err != nil {
				return exitWithError(out, err)
			}
			req.Service = selected
		}
	}

	workflow := workflows.NewLogsWorkflow(deps.Logger, newRuntimeEnvApplier(repoResolver), ports.NewLegacyUI(out))
	if err := workflow.Run(req); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}
