// Where: cli/internal/command/deploy_inputs_compose.go
// What: Compose-file normalization helpers for deploy inputs.
// Why: Keep path normalization concerns separate from prompt flow logic.
package command

import (
	"path/filepath"
	"strings"
)

func normalizeComposeFiles(files []string, baseDir string) []string {
	if len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed == "" {
			continue
		}
		path := trimmed
		if !filepath.IsAbs(path) && strings.TrimSpace(baseDir) != "" {
			path = filepath.Join(baseDir, path)
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		out = append(out, path)
		seen[path] = struct{}{}
	}
	return out
}
