// Where: cli/internal/helpers/project_finder.go
// What: Project directory discovery helpers.
// Why: Centralize generator.yml lookups for commands/resolver.
package helpers

import (
	"os"
	"path/filepath"
)

// ProjectDirFinder locates the nearest ancestor directory containing generator.yml.
type ProjectDirFinder func(start string) (string, bool)

// DefaultProjectDirFinder returns the stock finder that walks upward.
func DefaultProjectDirFinder() ProjectDirFinder {
	return func(start string) (string, bool) {
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
}
