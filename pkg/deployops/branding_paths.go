package deployops

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveBrandHomeDir(_ string) string {
	return defaultBrandHomeDir
}

func resolveRepoRoot(manifestPath, artifactRoot string) string {
	candidates := []string{artifactRoot, filepath.Dir(manifestPath)}
	for _, candidate := range candidates {
		if root, ok := findAncestorWithPath(candidate, ".git"); ok {
			return root
		}
	}
	if root := commonAncestorPath(artifactRoot, filepath.Dir(manifestPath)); root != "" {
		return root
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return filepath.Dir(manifestPath)
}

func findAncestorWithPath(start, name string) (string, bool) {
	current := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(current, name)); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func commonAncestorPath(first, second string) string {
	current := filepath.Clean(first)
	target := filepath.Clean(second)
	for {
		if hasPathPrefix(target, current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func hasPathPrefix(path, prefix string) bool {
	cleanPath := filepath.Clean(path)
	cleanPrefix := filepath.Clean(prefix)
	if cleanPath == cleanPrefix {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanPrefix+string(filepath.Separator))
}
