// Where: cli/internal/app/env.go
// What: Environment management commands.
// Why: Provide env list/create/use/remove for generator.yml and global config.
package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// EnvCmd groups all environment management subcommands including
// list, create, use, and remove operations.
type EnvCmd struct {
	List   EnvListCmd   `cmd:"" help:"List environments"`
	Create EnvCreateCmd `cmd:"" help:"Create environment"`
	Use    EnvUseCmd    `cmd:"" help:"Switch environment"`
	Remove EnvRemoveCmd `cmd:"" help:"Remove environment"`
}

type (
	EnvListCmd struct {
		Force bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
	EnvCreateCmd struct {
		Name  string `arg:"" help:"Environment name"`
		Force bool   `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
	EnvUseCmd struct {
		Name  string `arg:"" optional:"" help:"Environment name (interactive if omitted)"`
		Force bool   `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
	EnvRemoveCmd struct {
		Name  string `arg:"" help:"Environment name"`
		Force bool   `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
)

// envContext holds the resolved project and global configuration
// needed for environment operations.
type envContext struct {
	Project    projectConfig
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

	// Pre-calculate status for all environments
	type envInfo struct {
		Name   string
		Mode   string
		Status string
		Active bool
	}

	activeEnv := strings.TrimSpace(ctx.Project.Generator.App.LastEnv)
	var infos []envInfo

	for _, env := range ctx.Project.Generator.Environments {
		name := strings.TrimSpace(env.Name)
		if name == "" {
			continue
		}

		status := "unknown"
		if deps.DetectorFactory != nil {
			detector, err := deps.DetectorFactory(ctx.Project.Dir, name)
			if err == nil && detector != nil {
				if current, err := detector.Detect(); err == nil {
					status = string(current)
				}
			}
		}

		infos = append(infos, envInfo{
			Name:   name,
			Mode:   env.Mode,
			Status: status,
			Active: name == activeEnv,
		})
	}

	for _, info := range infos {
		marker := " "
		if info.Active {
			marker = "*"
		}

		color := "" // TODO: Add colors if terminal supports it
		fmt.Fprintf(out, "%s %s (%s) - %s%s\n", marker, info.Name, info.Mode, color, info.Status)
	}
	return 0
}

// runEnvCreate executes the 'env create' command which adds a new environment
// to the generator.yml configuration with an optional mode specifier.
func runEnvCreate(cli CLI, deps Dependencies, out io.Writer) int {
	rawName := strings.TrimSpace(cli.Env.Create.Name)
	if rawName == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}

	opts := newResolveOptions(cli.Env.Create.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	name, mode := splitEnvMode(rawName)
	if name == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}
	if mode == "" {
		mode = defaultMode()
	}
	if ctx.Project.Generator.Environments.Has(name) {
		fmt.Fprintln(out, "environment already exists")
		return 1
	}

	ctx.Project.Generator.Environments = append(ctx.Project.Generator.Environments, config.EnvironmentSpec{
		Name: name,
		Mode: mode,
	})
	if err := config.SaveGeneratorConfig(ctx.Project.GeneratorPath, ctx.Project.Generator); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Created environment '%s'\n", name)
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
		if !isTerminal(os.Stdin) {
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
		options := make([]selectOption, len(envs))
		for i, env := range envs {
			label := fmt.Sprintf("%s (%s)", env.Name, env.Mode)
			if env.Name == strings.TrimSpace(ctx.Project.Generator.App.LastEnv) {
				label = "* " + label
			}
			options[i] = selectOption{Label: label, Value: env.Name}
		}

		selected, err := selectFromList("Select environment", options)
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

	if ctx.ConfigPath == "" {
		return exitWithError(out, fmt.Errorf("global config path not available"))
	}

	ctx.Project.Generator.App.LastEnv = name
	if err := config.SaveGeneratorConfig(ctx.Project.GeneratorPath, ctx.Project.Generator); err != nil {
		return exitWithError(out, err)
	}

	cfg := normalizeGlobalConfig(ctx.Config)
	entry := cfg.Projects[ctx.Project.Name]
	entry.Path = ctx.Project.Dir
	entry.LastUsed = now(deps).Format(time.RFC3339)
	cfg.Projects[ctx.Project.Name] = entry
	if err := saveGlobalConfig(ctx.ConfigPath, cfg); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintf(os.Stderr, "Switched to '%s:%s'\n", ctx.Project.Name, name)
	fmt.Fprintf(out, "export ESB_ENV=%s\n", name)
	return 0
}

// parseIndex parses a 1-indexed selection string and returns the 0-indexed value.
func parseIndex(input string, maxVal int) (int, error) {
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err != nil {
		return 0, err
	}
	if idx < 1 || idx > maxVal {
		return 0, fmt.Errorf("index out of range")
	}
	return idx - 1, nil
}

// runEnvRemove executes the 'env remove' command which deletes an environment
// from generator.yml and updates the global configuration if necessary.
func runEnvRemove(cli CLI, deps Dependencies, out io.Writer) int {
	name := strings.TrimSpace(cli.Env.Remove.Name)
	if name == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}

	opts := newResolveOptions(cli.Env.Remove.Force)
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if !ctx.Project.Generator.Environments.Has(name) {
		fmt.Fprintln(out, "environment not found")
		return 1
	}
	if len(ctx.Project.Generator.Environments) <= 1 {
		fmt.Fprintln(out, "cannot remove the last environment")
		return 1
	}

	filtered := make(config.Environments, 0, len(ctx.Project.Generator.Environments)-1)
	for _, env := range ctx.Project.Generator.Environments {
		if strings.TrimSpace(env.Name) == name {
			continue
		}
		filtered = append(filtered, env)
	}
	ctx.Project.Generator.Environments = filtered
	if strings.TrimSpace(ctx.Project.Generator.App.LastEnv) == name {
		ctx.Project.Generator.App.LastEnv = ""
	}
	if err := config.SaveGeneratorConfig(ctx.Project.GeneratorPath, ctx.Project.Generator); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Removed environment '%s'\n", name)
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

	project, err := loadProjectConfig(projectDir)
	if err != nil {
		return envContext{}, err
	}
	path, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		return envContext{}, err
	}

	return envContext{
		Project:    project,
		Config:     cfg,
		ConfigPath: path,
	}, nil
}
