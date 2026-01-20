// Where: cli/internal/commands/project.go
// What: Project management commands.
// Why: Allow selecting and listing projects from global config.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// ProjectCmd groups all project management subcommands including
// list, use, and recent operations.
type ProjectCmd struct {
	List   ProjectListCmd   `cmd:"" help:"List projects" aliases:"ls"`
	Add    ProjectAddCmd    `cmd:"" help:"Add project"`
	Use    ProjectUseCmd    `cmd:"" help:"Switch project"`
	Remove ProjectRemoveCmd `cmd:"" help:"Remove project"`
	Recent ProjectRecentCmd `cmd:"" help:"Show recent projects"`
}

type (
	ProjectListCmd struct{}
	ProjectAddCmd  struct {
		Path string `arg:"" optional:"" help:"Directory path to add (default: .)"`
		Name string `help:"Project name" short:"n"`
	}
	ProjectRecentCmd struct{}
	ProjectUseCmd    struct {
		Name string `arg:"" optional:"" help:"Project name or index (interactive if omitted)"`
	}
	ProjectRemoveCmd struct {
		Name string `arg:"" optional:"" help:"Project name or index (interactive if omitted)"`
	}
)

// runProjectList executes the 'project list' command which displays
// all registered projects.
func runProjectList(_ CLI, deps Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault(deps)
	if err != nil {
		legacyUI(out).Warn(err.Error())
		return 1
	}
	ui := legacyUI(out)

	workflow := workflows.NewProjectListWorkflow()
	result, err := workflow.Run(workflows.ProjectListRequest{Config: cfg})
	if err != nil {
		return exitWithError(out, err)
	}

	if len(result.Projects) == 0 {
		ui.Info("📦 No projects registered.")
		return 0
	}

	for _, project := range result.Projects {
		if project.Active {
			ui.Info(fmt.Sprintf("📦  %s", project.Name))
			continue
		}
		ui.Info(fmt.Sprintf("    %s", project.Name))
	}
	return 0
}

// runProjectRecent executes the 'project recent' command which displays
// projects sorted by most recent usage with numbered indices.
func runProjectRecent(_ CLI, deps Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault(deps)
	if err != nil {
		legacyUI(out).Warn(err.Error())
		return 1
	}
	ui := legacyUI(out)

	workflow := workflows.NewProjectRecentWorkflow()
	result, err := workflow.Run(workflows.ProjectRecentRequest{Config: cfg})
	if err != nil {
		return exitWithError(out, err)
	}
	list := result.Projects
	if len(list) == 0 {
		ui.Info("no projects registered")
		return 0
	}

	for i, project := range list {
		ui.Info(fmt.Sprintf("%2d. 📦 %s", i+1, project.Name))
	}
	return 0
}

// runProjectUse executes the 'project use' command which switches
// the active project by name or recent index number. If no name is provided
// and running in a TTY, prompts the user to select from registered projects.
func runProjectUse(cli CLI, deps Dependencies, out io.Writer) int {
	path, cfg, err := loadGlobalConfigWithPath(deps)
	if err != nil {
		return exitWithError(out, err)
	}

	if len(cfg.Projects) == 0 {
		return exitWithSuggestion(out, "No projects registered.",
			[]string{"esb project add . -t <template>"})
	}

	selector := strings.TrimSpace(cli.Project.Use.Name)

	// Interactive selection if no name provided
	if selector == "" {
		// Check if interactive mode is available
		if !interaction.IsTerminal(os.Stdin) {
			var names []string
			for name := range cfg.Projects {
				names = append(names, name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Project name required (non-interactive mode).",
				[]string{"esb project use <name>"},
				names,
			)
		}

		// Build options for huh selector
		list := sortProjectsByRecent(cfg)
		options := make([]interaction.SelectOption, len(list))
		for i, project := range list {
			options[i] = interaction.SelectOption{Label: project.Name, Value: project.Name}
		}

		if deps.Prompter == nil {
			return exitWithError(out, fmt.Errorf("prompter not configured"))
		}
		selected, err := deps.Prompter.SelectValue("Select project", options)
		if err != nil {
			return exitWithError(out, err)
		}
		selector = selected
	}

	projectName, err := selectProject(cfg, selector)
	if err != nil {
		var names []string
		for name := range cfg.Projects {
			names = append(names, name)
		}
		return exitWithSuggestionAndAvailable(out,
			fmt.Sprintf("Project '%s' not found.", selector),
			[]string{"esb project use <name>", "esb project list"},
			names,
		)
	}

	workflow := workflows.NewProjectUseWorkflow()
	if err := workflow.Run(workflows.ProjectUseRequest{
		ProjectName:      projectName,
		GlobalConfig:     cfg,
		GlobalConfigPath: path,
		Now:              now(deps),
	}); err != nil {
		return exitWithError(out, err)
	}

	legacyUI(os.Stderr).Info(fmt.Sprintf("Switched to project '%s'", projectName))
	legacyUI(out).Info(fmt.Sprintf("export %s=%s", envutil.HostEnvKey(constants.HostSuffixProject), projectName))
	return 0
}

// runProjectRemove executes the 'project remove' command which deregisters
// a project from the global configuration.
func runProjectRemove(cli CLI, deps Dependencies, out io.Writer) int {
	path, cfg, err := loadGlobalConfigWithPath(deps)
	if err != nil {
		return exitWithError(out, err)
	}

	selector := cli.Project.Remove.Name
	if selector == "" {
		if !interaction.IsTerminal(os.Stdin) {
			var names []string
			for name := range cfg.Projects {
				names = append(names, name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Project name required (non-interactive mode).",
				[]string{"esb project remove <name>"},
				names,
			)
		}

		if deps.Prompter == nil {
			return exitWithError(out, fmt.Errorf("project name or index is required"))
		}

		list := sortProjectsByRecent(cfg)
		options := make([]string, len(list))
		for i, p := range list {
			options[i] = p.Name
		}

		selected, err := deps.Prompter.Select("Select project to remove", options)
		if err != nil {
			return exitWithError(out, err)
		}
		selector = selected
	}

	projectName, err := selectProject(cfg, selector)
	if err != nil {
		var names []string
		for name := range cfg.Projects {
			names = append(names, name)
		}
		return exitWithSuggestionAndAvailable(out,
			fmt.Sprintf("Project '%s' not found.", selector),
			[]string{"esb project remove <name>", "esb project list"},
			names,
		)
	}

	workflow := workflows.NewProjectRemoveWorkflow()
	if err := workflow.Run(workflows.ProjectRemoveRequest{
		ProjectName:      projectName,
		GlobalConfig:     cfg,
		GlobalConfigPath: path,
	}); err != nil {
		return exitWithError(out, err)
	}

	legacyUI(out).Info(fmt.Sprintf("Removed project '%s' from registered projects.", projectName))
	return 0
}

// runProjectAdd executes the 'project add' command which registers a project
// directory. If generator.yml is missing, it initializes it from a SAM template.
func runProjectAdd(cli CLI, deps Dependencies, out io.Writer) int {
	dir := cli.Project.Add.Path
	if dir == "" {
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return exitWithError(out, err)
	}

	generatorPath := filepath.Join(absDir, "generator.yml")
	if _, err := os.Stat(generatorPath); os.IsNotExist(err) {
		// New project initialization (old esb init behavior)
		template := cli.Template
		if template == "" {
			template = envutil.GetHostEnv(constants.HostSuffixTemplate)
		}
		if template == "" {
			// 1. Try to auto-detect standard template files
			for _, name := range []string{"template.yaml", "template.yml"} {
				p := filepath.Join(absDir, name)
				if _, err := os.Stat(p); err == nil {
					template = p
					legacyUI(out).Info(fmt.Sprintf("Detected template: %s", name))
					break
				}
			}

			// 2. Error if still not found
			if template == "" {
				return exitWithSuggestion(out, "Template path is required to initialize a new project.",
					[]string{"esb project add . --template <path>"})
			}
		}

		if template == "" {
			return exitWithSuggestion(out, "Template path is required to initialize a new project.",
				[]string{"esb project add . --template <path>"})
		}

		if cli.Project.Add.Name == "" {
			defaultName := filepath.Base(absDir)
			if interaction.IsTerminal(os.Stdin) && deps.Prompter != nil {
				name, err := deps.Prompter.Input("Project Name", []string{defaultName}) // Suggestion as slice
				if err != nil {
					return exitWithError(out, err)
				}
				if name != "" { // Only update if user entered something? Or if Prompter returns default?
					// Prompter.Input implementation usually returns empty if user just hits enter?
					// Wait, Input(title, suggestions).
					// If suggestions are provided, does it act as default?
					// My mock/impl of Input might differ.
					// Let's assume standard behavior: user types name or we use default.
					// Actually, common Prompter.Input(title, default) pattern...
					// Prompter.Input(title string, suggestions []string) (string, error)
					// It doesn't take a single default string, but suggestions.
					// Usually suggestions are for tab completion.
					// I need to interpret empty input as default manually if I want that.
					cli.Project.Add.Name = name
				}
				// If still empty, use default?
				if cli.Project.Add.Name == "" {
					cli.Project.Add.Name = defaultName
				}
			}
		}

		envs := splitEnvList(cli.EnvFlag)
		if len(envs) == 0 {
			if !interaction.IsTerminal(os.Stdin) || deps.Prompter == nil {
				return exitWithSuggestion(out, "Environment name is required for new projects.",
					[]string{"esb project add . -e dev:docker"})
			}

			// Interactive Prompt
			envName, err := deps.Prompter.Input("Environment Name (e.g., dev)", nil)
			if err != nil {
				return exitWithError(out, err)
			}
			if envName == "" {
				return exitWithError(out, fmt.Errorf("environment name is required"))
			}

			modeOptions := []interaction.SelectOption{
				{Label: "Docker (Standard)", Value: "docker"},
				{Label: "Containerd (Advanced)", Value: "containerd"},
				{Label: "Firecracker (MicroVM)", Value: "firecracker"},
			}
			mode, err := deps.Prompter.SelectValue("Runtime Mode", modeOptions)
			if err != nil {
				return exitWithError(out, err)
			}

			envs = []string{fmt.Sprintf("%s:%s", envName, mode)}
		}

		path, err := runInit(template, envs, cli.Project.Add.Name, deps.Prompter)
		if err != nil {
			return exitWithError(out, err)
		}
		generatorPath = path
		legacyUI(out).Info(fmt.Sprintf("Configuration saved to: %s", generatorPath))
	} else {
		legacyUI(out).Info(fmt.Sprintf("Found existing generator.yml in %s", absDir))
	}

	if err := registerProject(generatorPath, deps); err != nil {
		return exitWithError(out, err)
	}

	ui := legacyUI(out)
	ui.Info("Project registered successfully.")
	ui.Info("Next: esb build")
	return 0
}

// loadGlobalConfigOrDefault loads the global configuration or returns
// an empty config if not found.

// selectProject resolves a project selector (name or index) to a project name.
// Numeric selectors are 1-indexed and reference recent projects list.
func selectProject(cfg config.GlobalConfig, selector string) (string, error) {
	return workflows.SelectProject(cfg, selector)
}

// sortProjectsByRecent returns projects sorted by last-used timestamp.
func sortProjectsByRecent(cfg config.GlobalConfig) []workflows.RecentProject {
	return workflows.SortProjectsByRecent(cfg)
}
