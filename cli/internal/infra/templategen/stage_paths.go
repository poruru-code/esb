// Where: cli/internal/infra/templategen/stage_paths.go
// What: Resource and sitecustomize path resolution for staging.
// Why: Keep path selection logic independent from stage flow and layer handling.
package templategen

import (
	"path/filepath"
	"strings"

	"github.com/poruru-code/esb/cli/internal/domain/template"
)

func resolveResourcePath(baseDir, raw string) string {
	trimmed := strings.TrimLeft(raw, "/\\")
	if trimmed == "" {
		trimmed = raw
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func resolveSitecustomizeSource(ctx stageContext) string {
	source := ctx.SitecustomizePath
	if strings.TrimSpace(source) == "" {
		source = template.DefaultSitecustomizeSource
	}

	if filepath.IsAbs(source) {
		if fileExists(source) {
			return source
		}
		return ""
	}

	candidate := filepath.Clean(filepath.Join(ctx.BaseDir, source))
	if fileExists(candidate) {
		return candidate
	}

	candidate = filepath.Clean(filepath.Join(ctx.ProjectRoot, source))
	if fileExists(candidate) {
		return candidate
	}
	return ""
}
