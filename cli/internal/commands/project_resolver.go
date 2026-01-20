// Where: cli/internal/commands/project_resolver.go
// What: Resolve project directory from CLI flags and global config.
// Why: Ensure commands honor --template, project environment variable, and recent projects.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// projectSelection holds the resolved project directory and optional
// template override from CLI flags.
type projectSelection struct {
	Dir              string
	TemplateOverride string
}

// resolveProjectSelection determines the project directory based on CLI flags,
// project environment variable, or the most recently used project.
func resolveProjectSelection(cli CLI, _ Dependencies, opts resolveOptions) (projectSelection, error) {
	if strings.TrimSpace(cli.Template) != "" {
		absTemplate, err := filepath.Abs(cli.Template)
		if err != nil {
			return projectSelection{}, err
		}
		if _, err := os.Stat(absTemplate); err != nil {
			return projectSelection{}, err
		}
		return projectSelection{
			Dir:              filepath.Dir(absTemplate),
			TemplateOverride: absTemplate,
		}, nil
	}

	path, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		return projectSelection{}, err
	}

	appState, err := state.ResolveAppState(state.AppStateOptions{
		ProjectEnv:  envutil.GetHostEnv(constants.HostSuffixProject),
		Projects:    cfg.Projects,
		Force:       opts.Force,
		Interactive: opts.Interactive,
		Prompt:      opts.Prompt,
	})
	if err != nil {
		return projectSelection{}, err
	}
	if strings.TrimSpace(appState.ActiveProject) == "" {
		return projectSelection{}, fmt.Errorf("no active project; run 'esb project use <name>' first")
	}

	entry, ok := cfg.Projects[appState.ActiveProject]
	if !ok || strings.TrimSpace(entry.Path) == "" {
		return projectSelection{}, fmt.Errorf("Project not found: %s", appState.ActiveProject)
	}
	if dir, ok := findProjectDir(entry.Path); ok {
		return projectSelection{Dir: dir}, nil
	}

	return projectSelection{}, fmt.Errorf("project path not found: %s (config: %s)", entry.Path, path)
}

// findProjectDir searches upward from the given start directory to find
// a parent containing generator.yml. Returns the directory and success flag.
func findProjectDir(start string) (string, bool) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	dir := filepath.Clean(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, "generator.yml")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
