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
func runProjectList(_ CLI, _ Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	workflow := workflows.NewProjectListWorkflow()
	result, err := workflow.Run(workflows.ProjectListRequest{Config: cfg})
	if err != nil {
		return exitWithError(out, err)
	}

	if len(result.Projects) == 0 {
		fmt.Fprintln(out, "📦 No projects registered.")
		return 0
	}

	for _, project := range result.Projects {
		if project.Active {
			fmt.Fprintf(out, "📦  %s\n", project.Name)
			continue
		}
		fmt.Fprintf(out, "    %s\n", project.Name)
	}
	return 0
}

// runProjectRecent executes the 'project recent' command which displays
// projects sorted by most recent usage with numbered indices.
func runProjectRecent(_ CLI, _ Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	workflow := workflows.NewProjectRecentWorkflow()
	result, err := workflow.Run(workflows.ProjectRecentRequest{Config: cfg})
	if err != nil {
		return exitWithError(out, err)
	}
	list := result.Projects
	if len(list) == 0 {
		fmt.Fprintln(out, "no projects registered")
		return 0
	}

	for i, project := range list {
		fmt.Fprintf(out, "%2d. 📦 %s\n", i+1, project.Name)
	}
	return 0
}

// runProjectUse executes the 'project use' command which switches
// the active project by name or recent index number. If no name is provided
// and running in a TTY, prompts the user to select from registered projects.
func runProjectUse(cli CLI, deps Dependencies, out io.Writer) int {
	path, cfg, err := loadGlobalConfigWithPath()
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
		if !isTerminal(os.Stdin) {
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
		options := make([]selectOption, len(list))
		for i, project := range list {
			options[i] = selectOption{Label: project.Name, Value: project.Name}
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

	fmt.Fprintf(os.Stderr, "Switched to project '%s'\n", projectName)
	fmt.Fprintf(out, "export %s=%s\n", envutil.HostEnvKey(constants.HostSuffixProject), projectName)
	return 0
}

// runProjectRemove executes the 'project remove' command which deregisters
// a project from the global configuration.
func runProjectRemove(cli CLI, deps Dependencies, out io.Writer) int {
	path, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		return exitWithError(out, err)
	}

	selector := cli.Project.Remove.Name
	if selector == "" {
		if !isTerminal(os.Stdin) {
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

	fmt.Fprintf(out, "Removed project '%s' from registered projects.\n", projectName)
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
					fmt.Fprintf(out, "Detected template: %s\n", name)
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
			if isTerminal(os.Stdin) && deps.Prompter != nil {
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
					// Wait, Looking at Prompter interface in `command_context.go`:
					// Input(title string, suggestions []string) (string, error)
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
			if !isTerminal(os.Stdin) || deps.Prompter == nil {
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

			modeOptions := []selectOption{
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
		fmt.Fprintf(out, "Configuration saved to: %s\n", generatorPath)
	} else {
		fmt.Fprintf(out, "Found existing generator.yml in %s\n", absDir)
	}

	if err := registerProject(generatorPath, deps); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintf(out, "Project registered successfully.\n")
	fmt.Fprintln(out, "Next: esb build")
	return 0
}

// loadGlobalConfigOrDefault loads the global configuration or returns
// an empty config if not found.
func loadGlobalConfigOrDefault() (config.GlobalConfig, error) {
	path, err := config.GlobalConfigPath()
	if err != nil {
		return config.GlobalConfig{}, err
	}
	return loadGlobalConfig(path)
}

// loadGlobalConfigWithPath loads the global configuration and returns
// both the config path and the loaded configuration.
func loadGlobalConfigWithPath() (string, config.GlobalConfig, error) {
	path, err := config.GlobalConfigPath()
	if err != nil {
		return "", config.GlobalConfig{}, err
	}
	cfg, err := loadGlobalConfig(path)
	if err != nil {
		return "", config.GlobalConfig{}, err
	}
	return path, cfg, nil
}

// selectProject resolves a project selector (name or index) to a project name.
// Numeric selectors are 1-indexed and reference recent projects list.
func selectProject(cfg config.GlobalConfig, selector string) (string, error) {
	return workflows.SelectProject(cfg, selector)
}

// sortProjectsByRecent returns projects sorted by last-used timestamp.
func sortProjectsByRecent(cfg config.GlobalConfig) []workflows.RecentProject {
	return workflows.SortProjectsByRecent(cfg)
}
