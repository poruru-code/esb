// Where: cli/internal/commands/sync.go
// What: Sync command implementation.
// Why: Discover ports and provision resources without full orchestration.
package commands

import (
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runSync executes the 'sync' command: checks running containers,
// discovers/persists ports, and provisions SAM resources (DynamoDB/S3).
func runSync(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(false)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	ui := legacyUI(out)

	workflow := workflows.NewSyncWorkflow(
		deps.Sync.PortPublisher,
		deps.Sync.TemplateLoader,
		deps.Sync.TemplateParser,
		deps.Sync.Provisioner,
		ui,
	)

	req := workflows.SyncRequest{
		Context:      ctxInfo.Context,
		Env:          ctxInfo.Env,
		TemplatePath: resolvedTemplatePath(ctxInfo),
		Wait:         cli.Sync.Wait,
	}

	if _, err := workflow.Run(req); err != nil {
		return exitWithError(out, err)
	}

	return 0
}
