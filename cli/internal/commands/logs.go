// Where: cli/internal/commands/logs.go
// What: Logs command helpers.
// Why: Provide log access via docker compose with CLI flags.
package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// LogsRequest contains parameters for viewing container logs.
// It specifies follow mode, tail count, timestamps, and optional service filter.
func runLogs(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Logs.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	cmd, err := newLogsCommand(deps.Logs, repoResolver, out)
	if err != nil {
		return exitWithError(out, err)
	}

	req := workflows.LogsRequest{
		LogsRequest: ports.LogsRequest{
			Context:    ctxInfo.Context,
			Follow:     cli.Logs.Follow,
			Tail:       cli.Logs.Tail,
			Timestamps: cli.Logs.Timestamps,
			Service:    strings.TrimSpace(cli.Logs.Service),
		},
	}

	if req.Service == "" && isTerminal(os.Stdin) {
		services, err := deps.Logs.Logger.ListServices(req.LogsRequest)
		if err != nil {
			return exitWithError(out, err)
		}
		if len(services) > 0 {
			var options []selectOption
			options = append(options, selectOption{Label: "All services", Value: ""})
			for _, svc := range services {
				options = append(options, selectOption{Label: svc, Value: svc})
			}

			if deps.Prompter == nil {
				return exitWithError(out, fmt.Errorf("prompter not configured"))
			}
			selected, err := deps.Prompter.SelectValue("Select service to view logs", options)
			if err != nil {
				return exitWithError(out, err)
			}
			req.Service = selected
		}
	}

	if err := cmd.Run(req); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type logsCommand struct {
	logger     ports.Logger
	envApplier ports.RuntimeEnvApplier
	ui         ports.UserInterface
}

func newLogsCommand(deps LogsDeps, repoResolver func(string) (string, error), out io.Writer) (*logsCommand, error) {
	if deps.Logger == nil {
		return nil, fmt.Errorf("logs: not implemented")
	}
	return &logsCommand{
		logger:     deps.Logger,
		envApplier: helpers.NewRuntimeEnvApplier(repoResolver),
		ui:         ports.NewLegacyUI(out),
	}, nil
}

func (c *logsCommand) Run(req workflows.LogsRequest) error {
	return workflows.NewLogsWorkflow(c.logger, c.envApplier, c.ui).Run(req)
}
