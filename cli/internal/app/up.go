// Where: cli/internal/app/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package app

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// runUp executes the 'up' command which starts all services,
// optionally rebuilds images, provisions Lambda functions, and waits for readiness.
func runUp(cli CLI, deps Dependencies, out io.Writer) int {
	upDeps := deps.Up

	if upDeps.Upper == nil {
		fmt.Fprintln(out, "up: not implemented")
		return 1
	}
	if upDeps.Provisioner == nil {
		fmt.Fprintln(out, "up: provisioner not configured")
		return 1
	}

	if cli.Up.Reset && upDeps.Downer == nil {
		fmt.Fprintln(out, "up: downer not configured")
		return 1
	}
	if (cli.Up.Build || cli.Up.Reset) && upDeps.Builder == nil {
		fmt.Fprintln(out, "up: builder not configured")
		return 1
	}
	if upDeps.Parser == nil {
		fmt.Fprintln(out, "up: parser not configured")
		return 1
	}
	if cli.Up.Wait && upDeps.Waiter == nil {
		fmt.Fprintln(out, "up: waiter not configured")
		return 1
	}

	opts := newResolveOptions(cli.Up.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	if cli.Up.Reset && !cli.Up.Yes {
		if !isTerminal(os.Stdin) {
			return exitWithError(out, fmt.Errorf("up --reset requires --yes in non-interactive mode"))
		}
		confirmed, err := promptYesNo("Are you sure you want to continue?")
		if err != nil {
			return exitWithError(out, err)
		}
		if !confirmed {
			fmt.Fprintln(out, "Aborted.")
			return 1
		}
	}

	return runUpWithDeps(upDeps, deps.RepoResolver, ctxInfo, cli.Up, cli.EnvFile, out)
}

func runUpWithDeps(deps UpDeps, repoResolver func(string) (string, error), ctxInfo commandContext, flags UpCmd, envFile string, out io.Writer) int {
	envApplier := newRuntimeEnvApplier(repoResolver)
	workflow := workflows.NewUpWorkflow(
		deps.Builder,
		deps.Upper,
		deps.Downer,
		newPortPublisher(deps.PortDiscoverer),
		newCredentialManager(),
		newTemplateLoader(),
		newTemplateParser(deps.Parser),
		deps.Provisioner,
		deps.Waiter,
		envApplier,
		ports.NewLegacyUI(out),
	)

	request := workflows.UpRequest{
		Context:      ctxInfo.Context,
		Env:          ctxInfo.Env,
		TemplatePath: resolvedTemplatePath(ctxInfo),
		Detach:       flags.Detach,
		Wait:         flags.Wait,
		Build:        flags.Build,
		Reset:        flags.Reset,
		EnvFile:      envFile,
	}

	if _, err := workflow.Run(request); err != nil {
		return exitWithError(out, err)
	}

	return 0
}
