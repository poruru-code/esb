// Where: cli/internal/commands/command_context.go
// What: Shared context resolution for CLI commands.
// Why: Reduce duplicated selection/env/context setup across commands.
package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// exitWithError prints an error message to the output writer and returns
// exit code 1 for CLI error handling.
func exitWithError(out io.Writer, err error) int {
	legacyUI(out).Warn(fmt.Sprintf("✗ %v", err))
	return 1
}

// exitWithSuggestion prints an error with suggested next steps.
func exitWithSuggestion(out io.Writer, message string, suggestions []string) int {
	ui := legacyUI(out)
	ui.Warn(fmt.Sprintf("⚠️  %s", message))
	if len(suggestions) > 0 {
		ui.Info("")
		ui.Info("💡 Next steps:")
		for _, s := range suggestions {
			ui.Info(fmt.Sprintf("   - %s", s))
		}
	}
	return 1
}

// exitWithSuggestionAndAvailable prints an error with suggestions and available options.
func exitWithSuggestionAndAvailable(out io.Writer, message string, suggestions, available []string) int {
	ui := legacyUI(out)
	ui.Warn(fmt.Sprintf("⚠️  %s", message))
	if len(suggestions) > 0 {
		ui.Info("")
		ui.Info("💡 Next steps:")
		for _, s := range suggestions {
			ui.Info(fmt.Sprintf("   - %s", s))
		}
	}
	if len(available) > 0 {
		ui.Info("")
		ui.Info("🛠️  Available:")
		for _, a := range available {
			ui.Info(fmt.Sprintf("   - %s", a))
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

	project, err := loadProjectConfig(deps, projectDir)
	if err != nil {
		return commandContext{}, err
	}
	envState, err := state.ResolveProjectState(state.ProjectStateOptions{
		EnvFlag:         cli.EnvFlag,
		EnvVar:          envutil.GetHostEnv(constants.HostSuffixEnv),
		Config:          project.Generator,
		Force:           opts.Force,
		Interactive:     opts.Interactive,
		Prompt:          opts.Prompt,
		AllowMissingEnv: opts.AllowMissingEnv || opts.Interactive,
	})
	if err != nil {
		return commandContext{}, err
	}

	env := strings.TrimSpace(envState.ActiveEnv)
	if env == "" && opts.Interactive {
		// Project config is already loaded, but we need to check environments.
		// Re-loading or using existing project config if accessible.
		// Current logic loaded 'project' but didn't pass it fully here.
		// We can re-use 'project' variable loaded above.
		var options []interaction.SelectOption
		for _, e := range project.Generator.Environments {
			options = append(options, interaction.SelectOption{Label: fmt.Sprintf("%s (%s)", e.Name, e.Mode), Value: e.Name})
		}
		if len(options) > 0 {
			if deps.Prompter == nil {
				return commandContext{}, fmt.Errorf("prompter not configured")
			}
			selectedEnv, err := deps.Prompter.SelectValue("Select environment", options)
			if err != nil {
				return commandContext{}, err
			}
			env = selectedEnv
		}
	}

	if env == "" {
		if opts.AllowMissingEnv {
			return commandContext{Selection: selection, Env: "", Context: state.Context{ProjectDir: projectDir}}, nil
		}
		return commandContext{}, fmt.Errorf("no active environment; run 'esb env use <name>' first")
	}

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		return commandContext{}, err
	}

	return commandContext{Selection: selection, Env: env, Context: ctx}, nil
}
