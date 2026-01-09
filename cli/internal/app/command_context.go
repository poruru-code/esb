// Where: cli/internal/app/command_context.go
// What: Shared context resolution for CLI commands.
// Why: Reduce duplicated selection/env/context setup across commands.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func exitWithError(out io.Writer, err error) int {
	fmt.Fprintln(out, err)
	return 1
}

func resolvedTemplatePath(ctxInfo commandContext) string {
	if override := strings.TrimSpace(ctxInfo.Selection.TemplateOverride); override != "" {
		return override
	}
	return ctxInfo.Context.TemplatePath
}

type commandContext struct {
	Selection projectSelection
	Env       string
	Context   state.Context
}

func resolveCommandContext(cli CLI, deps Dependencies) (commandContext, error) {
	selection, err := resolveProjectSelection(cli, deps)
	if err != nil {
		return commandContext{}, err
	}
	projectDir := strings.TrimSpace(selection.Dir)
	if projectDir == "" {
		projectDir = "."
	}

	envDeps := deps
	envDeps.ProjectDir = projectDir
	env := resolveEnv(cli, envDeps)

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		return commandContext{}, err
	}

	return commandContext{Selection: selection, Env: env, Context: ctx}, nil
}
