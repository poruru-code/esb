// Where: cli/internal/app/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// UpRequest contains parameters for starting the environment.
// It includes the context information and flags for detached/wait modes.
type UpRequest struct {
	Context state.Context
	Detach  bool
	Wait    bool
	EnvFile string
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
	applyRuntimeEnv(ctx, deps.RepoResolver)

	templatePath := resolvedTemplatePath(ctxInfo)

	if cli.Up.Reset {
		if deps.Downer == nil {
			fmt.Fprintln(out, "up: downer not configured")
			return 1
		}
		printResetWarning(out)
		if !cli.Up.Yes {
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
		if err := deps.Downer.Down(ctx.ComposeProject, true); err != nil {
			return exitWithError(out, err)
		}
	}

	// Ensure authentication credentials are set (auto-generate if missing)
	creds := EnsureAuthCredentials()
	if creds.Generated {
		PrintGeneratedCredentials(out, creds)
	}

	if cli.Up.Build || cli.Up.Reset {
		if deps.Builder == nil {
			fmt.Fprintln(out, "up: builder not configured")
			return 1
		}

		request := manifest.BuildRequest{
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
		EnvFile: cli.EnvFile,
	}
	if err := deps.Upper.Up(request); err != nil {
		return exitWithError(out, err)
	}

	ports := DiscoverAndPersistPorts(ctx, deps.PortDiscoverer, out)

	// Provision resources
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return exitWithError(out, fmt.Errorf("failed to read template: %w", err)	)
	}

	if deps.Parser == nil {
		fmt.Fprintln(out, "up: parser not configured")
		return 1
	}

	parsed, err := deps.Parser.Parse(string(content), nil)
	if err != nil {
		return exitWithError(out, fmt.Errorf("failed to parse template: %w", err))
	}

	if err := deps.Provisioner.Apply(context.Background(), parsed.Resources, ctx.ComposeProject); err != nil {
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

	if ports != nil {
		PrintDiscoveredPorts(out, ports)
	}
	return 0
}

func printResetWarning(out io.Writer) {
	fmt.Fprintln(out, "WARNING! This will remove:")
	fmt.Fprintln(out, "  - all containers for the selected environment")
	fmt.Fprintln(out, "  - all volumes for the selected environment (DB/S3 data)")
	fmt.Fprintln(out, "  - rebuild images and restart services")
	fmt.Fprintln(out, "")
}
