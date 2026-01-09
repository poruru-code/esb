// Where: tools-go/internal/app/reset.go
// What: Reset command helpers.
// Why: Coordinate destructive reset flow with down + build.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

	projectDir := deps.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	env := resolveEnv(cli, deps)

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	applyModeEnv(ctx.Mode)

	templatePath := ctx.TemplatePath
	if cli.Template != "" {
		absTemplate, err := filepath.Abs(cli.Template)
		if err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		if _, err := os.Stat(absTemplate); err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		templatePath = absTemplate
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

	fmt.Fprintln(out, "reset complete")
	return 0
}
