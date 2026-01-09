// Where: cli/internal/app/project_resolver.go
// What: Resolve project directory from CLI flags and global config.
// Why: Ensure commands honor --template, current dir, and active project.
package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// projectSelection holds the resolved project directory and optional
// template override from CLI flags.
type projectSelection struct {
	Dir              string
	TemplateOverride string
}

// resolveProjectSelection determines the project directory based on CLI flags,
// current directory, or global config's active project.
func resolveProjectSelection(cli CLI, deps Dependencies) (projectSelection, error) {
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

	start := strings.TrimSpace(deps.ProjectDir)
	if start == "" {
		start = "."
	}
	if dir, ok := findProjectDir(start); ok {
		return projectSelection{Dir: dir}, nil
	}

	path, err := config.GlobalConfigPath()
	if err != nil {
		abs, _ := filepath.Abs(start)
		return projectSelection{Dir: abs}, nil
	}
	cfg, err := loadGlobalConfig(path)
	if err != nil {
		abs, _ := filepath.Abs(start)
		return projectSelection{Dir: abs}, nil
	}
	active := strings.TrimSpace(cfg.ActiveProject)
	if active != "" {
		if entry, ok := cfg.Projects[active]; ok {
			if strings.TrimSpace(entry.Path) != "" {
				if dir, ok := findProjectDir(entry.Path); ok {
					return projectSelection{Dir: dir}, nil
				}
			}
		}
	}

	abs, _ := filepath.Abs(start)
	return projectSelection{Dir: abs}, nil
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
