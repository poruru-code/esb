// Where: cli/internal/commands/env.go
// What: Environment management commands.
// Why: Provide env list/create/use/remove for generator.yml and global config.
package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// EnvCmd groups all environment management subcommands including
// list, create, use, and remove operations.
type EnvCmd struct {
	List   EnvListCmd   `cmd:"" help:"List environments"`
	Add    EnvAddCmd    `cmd:"" help:"Add environment"`
	Use    EnvUseCmd    `cmd:"" help:"Switch environment"`
	Remove EnvRemoveCmd `cmd:"" help:"Remove environment"`
}

type (
	EnvListCmd struct {
		Force bool `help:"Auto-unset invalid project/environment variables"`
	}
	EnvAddCmd struct {
		Name  string `arg:"" optional:"" help:"Environment name (interactive if omitted)"`
		Force bool   `help:"Auto-unset invalid project/environment variables"`
	}
	EnvUseCmd struct {
		Name  string `arg:"" optional:"" help:"Environment name (interactive if omitted)"`
		Force bool   `help:"Auto-unset invalid project/environment variables"`
	}
	EnvRemoveCmd struct {
		Name  string `arg:"" optional:"" help:"Environment name"`
		Force bool   `help:"Auto-unset invalid project/environment variables"`
	}
)

// envContext holds the resolved project and global configuration
// needed for environment operations.
type envContext struct {
	Project    helpers.ProjectConfig
	Config     config.GlobalConfig
	ConfigPath string
}

// runEnvList executes the 'env list' command which displays all environments
// defined in generator.yml, marking the active one with an asterisk.
func runEnvList(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Env.List.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ui := legacyUI(out)

	workflow := workflows.NewEnvListWorkflow(deps.DetectorFactory)
	result, err := workflow.Run(workflows.EnvListRequest{
		ProjectDir: ctx.Project.Dir,
		Generator:  ctx.Project.Generator,
	})
	if err != nil {
		return exitWithError(out, err)
	}

	for _, info := range result.Environments {
		marker := "    "
		if info.Active {
			marker = "🌐  "
		}

		statusIcon := "⚪"
		if info.Status == "running" {
			statusIcon = "🟢"
		}

		ui.Info(fmt.Sprintf("%s %s (%s) - %s %s", marker, info.Name, info.Mode, statusIcon, info.Status))
	}
	return 0
}

// runEnvAdd executes the 'env add' command which adds a new environment
// to the generator.yml configuration with an optional mode specifier.
func runEnvAdd(cli CLI, deps Dependencies, out io.Writer) int {
	rawName := strings.TrimSpace(cli.Env.Add.Name)

	opts := newResolveOptions(cli.Env.Add.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	isTTY := interaction.IsTerminal(os.Stdin)
	if rawName == "" {
		if !isTTY || deps.Prompter == nil {
			var names []string
			for _, env := range ctx.Project.Generator.Environments {
				names = append(names, env.Name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Environment name required.",
				[]string{"esb env create <name>"},
				names,
			)
		}

		// Prompt for name
		var suggestions []string
		for _, env := range ctx.Project.Generator.Environments {
			suggestions = append(suggestions, env.Name)
		}
		name, err := deps.Prompter.Input("Environment name", suggestions)
		if err != nil {
			return exitWithError(out, err)
		}
		rawName = strings.TrimSpace(name)
		if rawName == "" {
			return exitWithError(out, fmt.Errorf("environment name is required"))
		}
	}

	name, mode := splitEnvMode(rawName)
	if name == "" {
		return exitWithError(out, fmt.Errorf("invalid environment name"))
	}

	if mode == "" {
		if isTTY && deps.Prompter != nil {
			selected, err := deps.Prompter.Select("Select mode", []string{"docker", "containerd", "firecracker"})
			if err != nil {
				return exitWithError(out, err)
			}
			mode = selected
		} else {
			mode = defaultMode()
		}
	}

	if ctx.Project.Generator.Environments.Has(name) {
		return exitWithError(out, fmt.Errorf("environment %q already exists", name))
	}

	workflow := workflows.NewEnvAddWorkflow()
	if err := workflow.Run(workflows.EnvAddRequest{
		GeneratorPath: ctx.Project.GeneratorPath,
		Generator:     ctx.Project.Generator,
		Name:          name,
		Mode:          mode,
	}); err != nil {
		return exitWithError(out, err)
	}

	legacyUI(out).Info(fmt.Sprintf("Added environment '%s'", name))
	return 0
}

// runEnvUse executes the 'env use' command which switches the active environment
// and updates the global configuration. If no name is provided and running in a
// TTY, prompts the user to select from available environments.
func runEnvUse(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Env.Use.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	name := strings.TrimSpace(cli.Env.Use.Name)

	// Interactive selection if no name provided
	if name == "" {
		envs := ctx.Project.Generator.Environments
		if len(envs) == 0 {
			return exitWithSuggestion(out, "No environments defined.",
				[]string{"esb env create <name>"})
		}

		// Check if interactive mode is available
		if !interaction.IsTerminal(os.Stdin) {
			var names []string
			for _, env := range envs {
				names = append(names, env.Name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Environment name required (non-interactive mode).",
				[]string{"esb env use <name>"},
				names,
			)
		}

		// Build options for huh selector
		options := make([]interaction.SelectOption, len(envs))
		for i, env := range envs {
			label := fmt.Sprintf("%s (%s)", env.Name, env.Mode)
			if env.Name == strings.TrimSpace(ctx.Project.Generator.App.LastEnv) {
				label = "* " + label
			}
			options[i] = interaction.SelectOption{Label: label, Value: env.Name}
		}

		if deps.Prompter == nil {
			return exitWithError(out, fmt.Errorf("prompter not configured"))
		}
		selected, err := deps.Prompter.SelectValue("Select environment", options)
		if err != nil {
			return exitWithError(out, err)
		}
		name = selected
	}

	if !ctx.Project.Generator.Environments.Has(name) {
		var names []string
		for _, env := range ctx.Project.Generator.Environments {
			names = append(names, env.Name)
		}
		return exitWithSuggestionAndAvailable(out,
			fmt.Sprintf("Environment '%s' not found.", name),
			[]string{"esb env use <name>", "esb env list"},
			names,
		)
	}

	workflow := workflows.NewEnvUseWorkflow()
	if _, err := workflow.Run(workflows.EnvUseRequest{
		EnvName:          name,
		ProjectName:      ctx.Project.Name,
		ProjectDir:       ctx.Project.Dir,
		GeneratorPath:    ctx.Project.GeneratorPath,
		Generator:        ctx.Project.Generator,
		GlobalConfig:     ctx.Config,
		GlobalConfigPath: ctx.ConfigPath,
		Now:              now(deps),
	}); err != nil {
		return exitWithError(out, err)
	}

	legacyUI(os.Stderr).Info(fmt.Sprintf("Switched to '%s:%s'", ctx.Project.Name, name))
	legacyUI(out).Info(fmt.Sprintf("export %s=%s", envutil.HostEnvKey(constants.HostSuffixEnv), name))
	return 0
}

// runEnvRemove executes the 'env remove' command which deletes an environment
// from generator.yml and updates the global configuration if necessary.
func runEnvRemove(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Env.Remove.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	name := strings.TrimSpace(cli.Env.Remove.Name)
	if name == "" {
		envs := ctx.Project.Generator.Environments
		if len(envs) == 0 {
			return exitWithError(out, fmt.Errorf("no environments defined"))
		}

		if !interaction.IsTerminal(os.Stdin) {
			var names []string
			for _, env := range envs {
				names = append(names, env.Name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Environment name required (non-interactive mode).",
				[]string{"esb env remove <name>"},
				names,
			)
		}

		options := make([]interaction.SelectOption, len(envs))
		for i, env := range envs {
			options[i] = interaction.SelectOption{Label: fmt.Sprintf("%s (%s)", env.Name, env.Mode), Value: env.Name}
		}

		if deps.Prompter == nil {
			return exitWithError(out, fmt.Errorf("prompter not configured"))
		}
		selected, err := deps.Prompter.SelectValue("Select environment to remove", options)
		if err != nil {
			return exitWithError(out, err)
		}
		name = selected
	}

	workflow := workflows.NewEnvRemoveWorkflow()
	if err := workflow.Run(workflows.EnvRemoveRequest{
		Name:          name,
		GeneratorPath: ctx.Project.GeneratorPath,
		Generator:     ctx.Project.Generator,
	}); err != nil {
		ui := legacyUI(out)
		if errors.Is(err, workflows.ErrEnvNotFound) {
			ui.Warn("environment not found")
			return 1
		}
		if errors.Is(err, workflows.ErrEnvLast) {
			ui.Warn("cannot remove the last environment")
			return 1
		}
		ui.Warn(err.Error())
		return 1
	}

	legacyUI(out).Info(fmt.Sprintf("Removed environment '%s'", name))
	return 0
}

// resolveEnvContext loads the global and project configuration needed
// for environment management operations.
func resolveEnvContext(cli CLI, deps Dependencies, opts resolveOptions) (envContext, error) {
	selection, err := resolveProjectSelection(cli, deps, opts)
	if err != nil {
		return envContext{}, err
	}
	projectDir := selection.Dir
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}

	project, err := loadProjectConfig(deps, projectDir)
	if err != nil {
		return envContext{}, err
	}
	path, cfg, err := loadGlobalConfigWithPath(deps)
	if err != nil {
		return envContext{}, err
	}

	return envContext{
		Project:    project,
		Config:     cfg,
		ConfigPath: path,
	}, nil
}
