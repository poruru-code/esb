// Where: cli/internal/app/command_context.go
// What: Shared context resolution for CLI commands.
// Why: Reduce duplicated selection/env/context setup across commands.
package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// exitWithError prints an error message to the output writer and returns
// exit code 1 for CLI error handling.
func exitWithError(out io.Writer, err error) int {
	fmt.Fprintln(out, err)
	return 1
}

// resolvedTemplatePath returns the template path from the command context,
// preferring the CLI override if specified, otherwise using the context path.
func resolvedTemplatePath(ctxInfo commandContext) string {
	if override := strings.TrimSpace(ctxInfo.Selection.TemplateOverride); override != "" {
		return override
	}
	return ctxInfo.Context.TemplatePath
}

// commandContext holds the resolved project selection, environment, and state
// context needed for executing CLI commands.
type commandContext struct {
	Selection projectSelection
	Env       string
	Context   state.Context
}

// resolveCommandContext resolves the project selection, environment,
// and state context from CLI flags and dependencies.
func resolveCommandContext(cli CLI, deps Dependencies, opts resolveOptions) (commandContext, error) {
	selection, err := resolveProjectSelection(cli, deps, opts)
	if err != nil {
		return commandContext{}, err
	}
	projectDir := strings.TrimSpace(selection.Dir)
	if projectDir == "" {
		projectDir = "."
	}

	project, err := loadProjectConfig(projectDir)
	if err != nil {
		return commandContext{}, err
	}
	envState, err := state.ResolveProjectState(state.ProjectStateOptions{
		EnvFlag:     cli.EnvFlag,
		EnvVar:      os.Getenv("ESB_ENV"),
		Config:      project.Generator,
		Force:       opts.Force,
		Interactive: opts.Interactive,
		Prompt:      opts.Prompt,
	})
	if err != nil {
		return commandContext{}, err
	}
	env := strings.TrimSpace(envState.ActiveEnv)
	if env == "" {
		return commandContext{}, fmt.Errorf("No active environment. Run 'esb env use <name>' first.")
	}

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		return commandContext{}, err
	}

	return commandContext{Selection: selection, Env: env, Context: ctx}, nil
}
