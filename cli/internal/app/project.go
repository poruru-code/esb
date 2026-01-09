// Where: cli/internal/app/project.go
// What: Project management commands.
// Why: Allow selecting and listing projects from global config.
package app

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// ProjectCmd groups all project management subcommands including
// list, use, and recent operations.
type ProjectCmd struct {
	List   ProjectListCmd   `cmd:"" help:"List projects"`
	Use    ProjectUseCmd    `cmd:"" help:"Switch project"`
	Recent ProjectRecentCmd `cmd:"" help:"Show recent projects"`
}

type (
	ProjectListCmd   struct{}
	ProjectRecentCmd struct{}
	ProjectUseCmd    struct {
		Name string `arg:"" help:"Project name or index"`
	}
)

// recentProject holds project metadata for sorting by recent usage.
type recentProject struct {
	Name   string
	Entry  config.ProjectEntry
	UsedAt time.Time
}

// runProjectList executes the 'project list' command which displays
// all registered projects, marking the active one with an asterisk.
func runProjectList(_ CLI, _ Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if len(cfg.Projects) == 0 {
		fmt.Fprintln(out, "no projects registered")
		return 0
	}

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if name == cfg.ActiveProject {
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
// the active project by name or recent index number.
func runProjectUse(cli CLI, deps Dependencies, out io.Writer) int {
	selector := strings.TrimSpace(cli.Project.Use.Name)
	if selector == "" {
		fmt.Fprintln(out, "project name is required")
		return 1
	}

	path, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	projectName, err := selectProject(cfg, selector)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	updated := normalizeGlobalConfig(cfg)
	updated.ActiveProject = projectName
	entry := updated.Projects[projectName]
	entry.LastUsed = now(deps).Format(time.RFC3339)
	updated.Projects[projectName] = entry

	if err := saveGlobalConfig(path, updated); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Switched to project '%s'\n", projectName)
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
