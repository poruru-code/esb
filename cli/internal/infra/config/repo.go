// Where: cli/internal/infra/config/repo.go
// What: Repository discovery logic.
// Why: Centralize logic to find the ESB repository root from env, config, or file system.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

var (
	errRepoRootNotFound   = errors.New("repository root not found")
	errRepoRootNotFoundAt = errors.New("ESB repository root not found")
)

// ResolveRepoRoot determines the ESB repository root path.
// Priority order.
// 1. Brand-prefixed REPO environment variable (validated as root or searched upward).
// 2. Upward search for docker-compose.{mode}.yml from startDir.
// 3. repo_path in global config (~/.esb/config.yaml) (validated as root or searched upward).
func ResolveRepoRoot(startDir string) (string, error) {
	// 1. Try environment variable
	repo, err := envutil.GetHostEnv(constants.HostSuffixRepo)
	if err != nil {
		return "", fmt.Errorf("get host env %s: %w", constants.HostSuffixRepo, err)
	}
	if repo := strings.TrimSpace(repo); repo != "" {
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

	key, err := envutil.HostEnvKey(constants.HostSuffixRepo)
	if err != nil {
		return "", fmt.Errorf("resolve host env key for repo: %w", err)
	}
	return "", fmt.Errorf("%w: run from repo root or set %s", errRepoRootNotFound, key)
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
