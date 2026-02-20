// Where: cli/internal/domain/config/output.go
// What: Pure output path helpers for deploy summaries.
// Why: Keep output path logic consistent and reusable.
package config

import (
	"path/filepath"
	"strings"

	"github.com/poruru-code/esb/cli/internal/meta"
)

// NormalizeOutputDir normalizes the output directory name.
func NormalizeOutputDir(outputDir string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return meta.OutputDir
	}
	return trimmed
}

// ResolveOutputSummary resolves the final output path shown in deploy summaries.
func ResolveOutputSummary(templatePath, outputDir, env string) string {
	baseDir := filepath.Dir(templatePath)
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return filepath.Join(baseDir, meta.OutputDir, env)
	}
	path := trimmed
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Join(filepath.Clean(path), env)
}
