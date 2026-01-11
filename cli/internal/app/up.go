// Where: cli/internal/app/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// UpRequest contains parameters for starting the environment.
// It includes the context information and flags for detached/wait modes.
type UpRequest struct {
	Context state.Context
	Detach  bool
	Wait    bool
}

// Upper defines the interface for starting the environment.
// Implementations use Docker Compose to bring up the services.
type Upper interface {
	Up(request UpRequest) error
}

// runUp executes the 'up' command which starts all services,
// optionally rebuilds images, provisions Lambda functions, and waits for readiness.
func runUp(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Upper == nil {
		fmt.Fprintln(out, "up: not implemented")
		return 1
	}
	if deps.Provisioner == nil {
		fmt.Fprintln(out, "up: provisioner not configured")
		return 1
	}

	opts := newResolveOptions(cli.Up.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context
	applyRuntimeEnv(ctx)

	templatePath := resolvedTemplatePath(ctxInfo)

	if cli.Up.Build {
		if deps.Builder == nil {
			fmt.Fprintln(out, "up: builder not configured")
			return 1
		}

		request := BuildRequest{
			ProjectDir:   ctx.ProjectDir,
			TemplatePath: templatePath,
			Env:          ctxInfo.Env,
		}
		if err := deps.Builder.Build(request); err != nil {
			return exitWithError(out, err)
		}
	}

	request := UpRequest{
		Context: ctx,
		Detach:  cli.Up.Detach,
		Wait:    cli.Up.Wait,
	}
	if err := deps.Upper.Up(request); err != nil {
		return exitWithError(out, err)
	}

	discoverAndPersistPorts(ctx, deps.PortDiscoverer, out)

	if err := deps.Provisioner.Provision(ProvisionRequest{
		TemplatePath:   templatePath,
		ProjectDir:     ctx.ProjectDir,
		Env:            ctxInfo.Env,
		ComposeProject: ctx.ComposeProject,
		Mode:           ctx.Mode,
	}); err != nil {
		return exitWithError(out, err)
	}

	if cli.Up.Wait {
		if deps.Waiter == nil {
			fmt.Fprintln(out, "up: waiter not configured")
			return 1
		}
		if err := deps.Waiter.Wait(ctx); err != nil {
			return exitWithError(out, err)
		}
	}

	fmt.Fprintln(out, "✓ Up complete")
	fmt.Fprintln(out, "Next:")
	fmt.Fprintln(out, "  esb logs <service>  # View logs")
	fmt.Fprintln(out, "  esb down            # Stop environment")
	return 0
}
