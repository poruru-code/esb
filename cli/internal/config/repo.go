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
// 1. ESB_REPO environment variable
// 2. repo_path in global config (~/.esb/config.yaml)
// 3. Upward search for docker-compose.yml from startDir
func ResolveRepoRoot(startDir string) (string, error) {
	// 1. Try environment variable
	if repo := os.Getenv("ESB_REPO"); repo != "" {
		if info, err := os.Stat(repo); err == nil && info.IsDir() {
			return repo, nil
		}
	}

	// 2. Try global configuration
	if cfgPath, err := GlobalConfigPath(); err == nil {
		if cfg, err := LoadGlobalConfig(cfgPath); err == nil && cfg.RepoPath != "" {
			if info, err := os.Stat(cfg.RepoPath); err == nil && info.IsDir() {
				return cfg.RepoPath, nil
			}
		}
	}

	// 3. Search upwards from start directory (dev mode / inside repo)
	if startDir != "" {
		dir := filepath.Clean(startDir)
		for {
			if dir == "" || dir == string(filepath.Separator) {
				break
			}
			path := filepath.Join(dir, "docker-compose.yml")
			if _, err := os.Stat(path); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return "", fmt.Errorf("ESB repository root not found. Please run 'esb config set-repo <path>' or set ESB_REPO.") //nolint:revive
}
