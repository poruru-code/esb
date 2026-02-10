// Where: cli/internal/infra/templategen/stage_python.go
// What: Python layer layout helpers for staging.
// Why: Keep Python-specific decisions isolated from generic staging flow.
package templategen

import (
	"path/filepath"
	"strings"
)

// shouldNestPython returns true when a Python layer lacks an explicit python/
// layout and therefore must be nested to satisfy the runtime expectation.
func shouldNestPython(nest bool, sourceDir string) bool {
	if !nest {
		return false
	}
	if sourceDir == "" {
		return false
	}
	return !containsPythonLayout(sourceDir)
}

// containsPythonLayout checks for python/ or site-packages/ at the root level.
func containsPythonLayout(dir string) bool {
	return dirExists(filepath.Join(dir, "python")) || dirExists(filepath.Join(dir, "site-packages"))
}

func ensureSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
