// Where: cli/internal/command/deploy_template_discovery.go
// What: Template path normalization and candidate discovery helpers.
// Why: Keep file-system specific behavior separate from prompt interactions.
package command

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/meta"
)

func normalizeTemplatePath(path string) (string, error) {
	if path == "" {
		return "", errTemplatePathEmpty
	}

	expanded, err := expandHomePath(path)
	if err != nil {
		return "", fmt.Errorf("expand home path: %w", err)
	}

	cleaned := filepath.Clean(expanded)
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) && !filepath.IsAbs(cleaned) {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				if repoRoot, repoErr := config.ResolveRepoRoot(cwd); repoErr == nil {
					candidate := filepath.Join(repoRoot, cleaned)
					if altInfo, altErr := os.Stat(candidate); altErr == nil {
						cleaned = candidate
						info = altInfo
						err = nil
					}
				}
			}
		}
		if err != nil {
			return "", fmt.Errorf("stat template path: %w", err)
		}
	}

	if !info.IsDir() {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("resolve template path: %w", err)
		}
		return abs, nil
	}

	for _, name := range []string{"template.yaml", "template.yml"} {
		candidate := filepath.Join(cleaned, name)
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return "", fmt.Errorf("resolve template path: %w", err)
			}
			return abs, nil
		}
	}

	return "", fmt.Errorf("%w: %s", errTemplateNotFound, cleaned)
}

// expandHomePath expands ~ to the user's home directory.
func expandHomePath(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	if len(path) == 1 || path[1] == '/' || path[1] == filepath.Separator {
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

func discoverTemplateCandidates() []string {
	candidates := []string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		return candidates
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".git" || name == meta.OutputDir || name == "node_modules" || name == ".venv" {
			continue
		}
		if name == "__pycache__" || name == ".pytest_cache" || name == ".mypy_cache" {
			continue
		}
		if !hasTemplateFile(name) {
			continue
		}
		candidates = append(candidates, name)
	}
	sort.Strings(candidates)
	return candidates
}

func hasTemplateFile(dir string) bool {
	for _, name := range []string{"template.yaml", "template.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}
