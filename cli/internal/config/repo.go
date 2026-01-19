// Where: cli/internal/config/repo.go
// What: Repository discovery logic.
// Why: Centralize logic to find the ESB repository root from env, config, or file system.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

// ResolveRepoRoot determines the ESB repository root path.
// Priority:
// 1. Brand-prefixed REPO environment variable (validated as root or searched upward)
// 2. Upward search for docker-compose.yml from startDir
// 3. repo_path in global config (~/.esb/config.yaml) (validated as root or searched upward)
func ResolveRepoRoot(startDir string) (string, error) {
	// 1. Try environment variable
	if repo := strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixRepo)); repo != "" {
		if root, ok := findRepoRoot(repo); ok {
			return root, nil
		}
	}

	// 2. Search upwards from start directory (dev mode / inside repo)
	if startDir != "" {
		if root, ok := findRepoRoot(startDir); ok {
			return root, nil
		}
	}

	// 3. Try global configuration
	if cfgPath, err := GlobalConfigPath(); err == nil {
		if cfg, err := LoadGlobalConfig(cfgPath); err == nil && cfg.RepoPath != "" {
			if root, ok := findRepoRoot(cfg.RepoPath); ok {
				return root, nil
			}
		}
	}

	return "", fmt.Errorf("repository root not found. Please run 'esb config set-repo <path>' or set %s.", envutil.HostEnvKey(constants.HostSuffixRepo)) //nolint:revive
}

// ResolveRepoRootFromPath determines the ESB repository root path using only the supplied path.
func ResolveRepoRootFromPath(path string) (string, error) {
	if root, ok := findRepoRoot(path); ok {
		return root, nil
	}
	return "", fmt.Errorf("ESB repository root not found at %s", path)
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
