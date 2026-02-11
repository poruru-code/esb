// Where: cli/internal/infra/config/repo.go
// What: Repository discovery logic.
// Why: Centralize logic to find the ESB repository root from the current working directory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	errRepoRootNotFound   = errors.New("repository root not found")
	errRepoRootNotFoundAt = errors.New("ESB repository root not found")
)

// ResolveRepoRoot determines the ESB repository root path.
// If startDir is empty, it falls back to the current working directory.
func ResolveRepoRoot(startDir string) (string, error) {
	base := strings.TrimSpace(startDir)
	if base == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("%w: run from repo root", errRepoRootNotFound)
		}
		base = cwd
	}
	if root, ok := findRepoRoot(base); ok {
		return root, nil
	}
	return "", fmt.Errorf("%w: run from repo root", errRepoRootNotFound)
}

// ResolveRepoRootFromPath determines the ESB repository root path using only the supplied path.
func ResolveRepoRootFromPath(path string) (string, error) {
	if root, ok := findRepoRoot(path); ok {
		return root, nil
	}
	return "", fmt.Errorf("%w at %s", errRepoRootNotFoundAt, path)
}

// findRepoRoot searches upward from the given path to find
// a directory containing docker-compose.{mode}.yml.
func findRepoRoot(path string) (string, bool) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	markers := []string{
		"docker-compose.docker.yml",
		"docker-compose.containerd.yml",
	}

	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
