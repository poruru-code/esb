// Where: cli/internal/infra/templategen/generate_paths.go
// What: Path resolution helpers for template generation outputs.
// Why: Keep filesystem path logic separate from generation orchestration.
package templategen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru-code/esb/cli/internal/meta"
)

// resolveTemplatePath determines the absolute path to the SAM template.
func resolveTemplatePath(samTemplate, projectRoot string) (string, error) {
	if strings.TrimSpace(samTemplate) == "" {
		return "", fmt.Errorf("sam_template is required")
	}
	path, err := expandHomePath(strings.TrimSpace(samTemplate))
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, path)
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func expandHomePath(path string) (string, error) {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ResolveTemplatePath determines the absolute path to the SAM template.
func ResolveTemplatePath(samTemplate, projectRoot string) (string, error) {
	return resolveTemplatePath(samTemplate, projectRoot)
}

// resolveOutputDir returns the absolute output directory where artifacts will be staged.
func resolveOutputDir(outputDir, baseDir string) string {
	normalized := normalizeOutputDir(outputDir)
	path := normalized
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

// ResolveOutputDir returns the absolute output directory where artifacts are staged.
func ResolveOutputDir(outputDir, baseDir string) string {
	return resolveOutputDir(outputDir, baseDir)
}

// resolveConfigPath chooses where to write config files (functions/routing).
func resolveConfigPath(explicit, baseDir, outputDir, name string) string {
	if strings.TrimSpace(explicit) == "" {
		return filepath.Join(outputDir, "config", name)
	}
	path := explicit
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

func normalizeOutputDir(outputDir string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return meta.OutputDir
	}
	return trimmed
}
