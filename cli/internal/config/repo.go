// Where: cli/internal/config/repo.go
// What: Repository discovery logic.
// Why: Centralize logic to find the ESB repository root from env, config, or file system.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRepoRoot determines the ESB repository root path.
// Priority:
// 1. ESB_REPO environment variable (validated as root or searched upward)
// 2. repo_path in global config (~/.esb/config.yaml) (validated as root or searched upward)
// 3. Upward search for docker-compose.yml from startDir
func ResolveRepoRoot(startDir string) (string, error) {
	// 1. Try environment variable
	if repo := os.Getenv("ESB_REPO"); repo != "" {
		if root, ok := findRepoRoot(repo); ok {
			return root, nil
		}
	}

	// 2. Try global configuration
	if cfgPath, err := GlobalConfigPath(); err == nil {
		if cfg, err := LoadGlobalConfig(cfgPath); err == nil && cfg.RepoPath != "" {
			if root, ok := findRepoRoot(cfg.RepoPath); ok {
				return root, nil
			}
		}
	}

	// 3. Search upwards from start directory (dev mode / inside repo)
	if startDir != "" {
		if root, ok := findRepoRoot(startDir); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("ESB repository root not found. Please run 'esb config set-repo <path>' or set ESB_REPO.") //nolint:revive
}

// findRepoRoot searches upward from the given path to find
// a directory containing docker-compose.yml.
func findRepoRoot(path string) (string, bool) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
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
