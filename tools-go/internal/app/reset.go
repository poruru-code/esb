// Where: tools-go/internal/app/reset.go
// What: Reset command helpers.
// Why: Coordinate destructive reset flow with down + build.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/tools-go/internal/state"
)

func runReset(cli CLI, deps Dependencies, out io.Writer) int {
	if !cli.Reset.Yes {
		fmt.Fprintln(out, "reset requires confirmation (--yes)")
		return 1
	}
	if deps.Downer == nil || deps.Builder == nil || deps.Upper == nil {
		fmt.Fprintln(out, "reset: not implemented")
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
	applyUpEnv(ctx)

	templatePath := ctx.TemplatePath
	if selection.TemplateOverride != "" {
		templatePath = selection.TemplateOverride
	}

	if err := deps.Downer.Down(ctx.ComposeProject, true); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	request := BuildRequest{
		ProjectDir:   ctx.ProjectDir,
		TemplatePath: templatePath,
		Env:          env,
	}
	if err := deps.Builder.Build(request); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if err := deps.Upper.Up(UpRequest{Context: ctx, Detach: true}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	discoverAndPersistPorts(ctx, deps.PortDiscoverer, out)

	fmt.Fprintln(out, "reset complete")
	return 0
}
