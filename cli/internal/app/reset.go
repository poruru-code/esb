// Where: cli/internal/app/reset.go
// What: Reset command helpers.
// Why: Coordinate destructive reset flow with down + build.
package app

import (
	"fmt"
	"io"
)

// runReset executes the 'reset' command which performs a destructive reset
// by stopping containers, removing volumes, rebuilding images, and restarting.
func runReset(cli CLI, deps Dependencies, out io.Writer) int {
	if !cli.Reset.Yes {
		fmt.Fprintln(out, "reset requires confirmation (--yes)")
		return 1
	}
	if deps.Downer == nil || deps.Builder == nil || deps.Upper == nil {
		fmt.Fprintln(out, "reset: not implemented")
		return 1
	}

	opts := newResolveOptions(cli.Reset.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context
	applyRuntimeEnv(ctx)

	templatePath := resolvedTemplatePath(ctxInfo)

	if err := deps.Downer.Down(ctx.ComposeProject, true); err != nil {
		return exitWithError(out, err)
	}

	request := BuildRequest{
		ProjectDir:   ctx.ProjectDir,
		TemplatePath: templatePath,
		Env:          ctxInfo.Env,
	}
	if err := deps.Builder.Build(request); err != nil {
		return exitWithError(out, err)
	}

	if err := deps.Upper.Up(UpRequest{Context: ctx, Detach: true}); err != nil {
		return exitWithError(out, err)
	}
	discoverAndPersistPorts(ctx, deps.PortDiscoverer, out)

	fmt.Fprintln(out, "reset complete")
	return 0
}
