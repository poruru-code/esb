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

// Prompter defines the interface for interactive user input and selection.
type Prompter interface {
	Input(title string, suggestions []string) (string, error)
	InputPath(title string) (string, error)
	Select(title string, options []string) (string, error)
}

type mockPrompter struct {
	inputFn     func(title string, suggestions []string) (string, error)
	inputPathFn func(title string) (string, error)
	selectFn    func(title string, options []string) (string, error)
}

func (m mockPrompter) Input(title string, suggestions []string) (string, error) {
	if m.inputFn != nil {
		return m.inputFn(title, suggestions)
	}
	return "", nil
}

func (m mockPrompter) InputPath(title string) (string, error) {
	if m.inputPathFn != nil {
		return m.inputPathFn(title)
	}
	return "", nil
}

func (m mockPrompter) Select(title string, options []string) (string, error) {
	if m.selectFn != nil {
		return m.selectFn(title, options)
	}
	return "", nil
}

// exitWithError prints an error message to the output writer and returns
// exit code 1 for CLI error handling.
func exitWithError(out io.Writer, err error) int {
	fmt.Fprintf(out, "✗ %v\n", err)
	return 1
}

// exitWithSuggestion prints an error with suggested next steps.
func exitWithSuggestion(out io.Writer, message string, suggestions []string) int {
	fmt.Fprintf(out, "✗ %s\n", message)
	if len(suggestions) > 0 {
		fmt.Fprintln(out, "\nNext steps:")
		for _, s := range suggestions {
			fmt.Fprintf(out, "  %s\n", s)
		}
	}
	return 1
}

// exitWithSuggestionAndAvailable prints an error with suggestions and available options.
func exitWithSuggestionAndAvailable(out io.Writer, message string, suggestions, available []string) int {
	fmt.Fprintf(out, "✗ %s\n", message)
	if len(suggestions) > 0 {
		fmt.Fprintln(out, "\nNext steps:")
		for _, s := range suggestions {
			fmt.Fprintf(out, "  %s\n", s)
		}
	}
	if len(available) > 0 {
		fmt.Fprintln(out, "\nAvailable:")
		for _, a := range available {
			fmt.Fprintf(out, "  - %s\n", a)
		}
	}
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
		return commandContext{}, fmt.Errorf("no active environment; run 'esb env use <name>' first")
	}

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		return commandContext{}, err
	}

	return commandContext{Selection: selection, Env: env, Context: ctx}, nil
}
