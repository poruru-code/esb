// Where: cli/internal/app/project.go
// What: Project management commands.
// Why: Allow selecting and listing projects from global config.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
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

// recentProject holds project metadata for sorting by recent usage.
type recentProject struct {
	Name   string
	Entry  config.ProjectEntry
	UsedAt time.Time
}

// runProjectList executes the 'project list' command which displays
// all registered projects.
func runProjectList(_ CLI, _ Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if len(cfg.Projects) == 0 {
		fmt.Fprintln(out, "No projects registered.")
		return 0
	}

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	activeProject := os.Getenv("ESB_PROJECT")

	for _, name := range names {
		if name == activeProject {
			fmt.Fprintf(out, "* %s\n", name)
			continue
		}
		fmt.Fprintln(out, name)
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

	list := sortProjectsByRecent(cfg)
	if len(list) == 0 {
		fmt.Fprintln(out, "no projects registered")
		return 0
	}

	for i, project := range list {
		fmt.Fprintf(out, "%d. %s\n", i+1, project.Name)
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

		selected, err := selectFromList("Select project", options)
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

	updated := normalizeGlobalConfig(cfg)
	entry := updated.Projects[projectName]
	entry.LastUsed = now(deps).Format(time.RFC3339)
	updated.Projects[projectName] = entry

	if err := saveGlobalConfig(path, updated); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintf(os.Stderr, "Switched to project '%s'\n", projectName)
	fmt.Fprintf(out, "export ESB_PROJECT=%s\n", projectName)
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

	delete(cfg.Projects, projectName)
	if err := saveGlobalConfig(path, cfg); err != nil {
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
			// 1. Try to auto-detect standard template files
			for _, name := range []string{"template.yaml", "template.yml"} {
				p := filepath.Join(absDir, name)
				if _, err := os.Stat(p); err == nil {
					template = p
					fmt.Fprintf(out, "Detected template: %s\n", name)
					break
				}
			}

			// 2. Prompt user if still not found
			if template == "" {
				var err error
				if deps.Prompter != nil {
					template, err = deps.Prompter.InputPath("Template path (e.g. template.yaml)")
				} else if isTerminal(os.Stdin) {
					template, err = promptLine("Template path (e.g. template.yaml)")
				} else {
					// Fallback for non-terminal environments
					return exitWithSuggestion(out, "Template path is required to initialize a new project.",
						[]string{"esb project add . --template <path>"})
				}

				if err != nil {
					return exitWithError(out, err)
				}
			}
		}

		if template == "" {
			return exitWithSuggestion(out, "Template path is required to initialize a new project.",
				[]string{"esb project add . --template <path>"})
		}

		envs := splitEnvList(cli.EnvFlag)
		if len(envs) == 0 {
			// Use prompter if available
			var input string
			var err error
			if deps.Prompter != nil {
				input, err = deps.Prompter.Input("Environment name (e.g. dev:docker, prod:containerd)", nil)
			} else if isTerminal(os.Stdin) {
				input, err = promptLine("Environment name (e.g. dev:docker, prod:containerd)")
			} else {
				return exitWithError(out, fmt.Errorf("environment name is required"))
			}

			if err != nil {
				return exitWithError(out, err)
			}
			envs = splitEnvList(input)
		}

		path, err := runInit(template, envs, cli.Project.Add.Name)
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
	if len(cfg.Projects) == 0 {
		return "", fmt.Errorf("no projects registered")
	}

	if index, err := strconv.Atoi(selector); err == nil {
		if index <= 0 {
			return "", fmt.Errorf("invalid project index")
		}
		list := sortProjectsByRecent(cfg)
		if index > len(list) {
			return "", fmt.Errorf("project index out of range")
		}
		return list[index-1].Name, nil
	}

	if _, ok := cfg.Projects[selector]; !ok {
		return "", fmt.Errorf("project not found")
	}
	return selector, nil
}

// sortProjectsByRecent returns projects sorted by last-used timestamp,
// with most recently used first. Ties are broken alphabetically.
func sortProjectsByRecent(cfg config.GlobalConfig) []recentProject {
	projects := make([]recentProject, 0, len(cfg.Projects))
	for name, entry := range cfg.Projects {
		projects = append(projects, recentProject{
			Name:   name,
			Entry:  entry,
			UsedAt: parseLastUsed(entry.LastUsed),
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].UsedAt.Equal(projects[j].UsedAt) {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].UsedAt.After(projects[j].UsedAt)
	})
	return projects
}

// parseLastUsed parses an RFC3339 timestamp string into a time.Time.
// Returns zero time if the string is empty or invalid.
func parseLastUsed(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
