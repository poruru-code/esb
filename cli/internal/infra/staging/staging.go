// Where: cli/internal/infra/staging/staging.go
// What: Shared helpers for staging directory layout.
// Why: Keep builder and runtime components aligned on where staged configs land.
package staging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/meta"
)

// ComposeProjectKey returns a filesystem-safe staging key for the provided
// compose project, falling back to a predictable value when the input is empty.
func ComposeProjectKey(composeProject, env string) string {
	if key := strings.TrimSpace(composeProject); key != "" {
		return key
	}
	if env = strings.TrimSpace(env); env != "" {
		return fmt.Sprintf("%s-%s", meta.Slug, strings.ToLower(env))
	}
	return meta.Slug
}

// CacheKey returns a stable key for staging caches based on project + env.
func CacheKey(composeProject, env string) string {
	return stageKey(composeProject, env)
}

func stageKey(composeProject, env string) string {
	key := ComposeProjectKey(composeProject, env)
	seed := strings.TrimSpace(key)
	if env = strings.TrimSpace(env); env != "" {
		seed = fmt.Sprintf("%s:%s", seed, strings.ToLower(env))
	}
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%s-%s", key, hex.EncodeToString(sum[:4]))
}

// RootDir returns the absolute cache root for staging assets.
// It prefers project-scoped staging next to the template directory and requires
// that location to be writable.
func RootDir(templatePath string) (string, error) {
	if override := strings.TrimSpace(getHostEnv(constants.HostSuffixStagingDir)); override != "" {
		root, err := absPath(override)
		if err != nil {
			return "", err
		}
		return ensureDir(root)
	}
	if override := strings.TrimSpace(getHostEnv(constants.HostSuffixStagingHome)); override != "" {
		root, err := absPath(filepath.Join(override, "staging"))
		if err != nil {
			return "", err
		}
		return ensureDir(root)
	}

	if templatePath != "" {
		templateDir := filepath.Dir(templatePath)
		if abs, err := filepath.Abs(templateDir); err == nil {
			templateDir = abs
		}
		root := filepath.Join(templateDir, meta.OutputDir, "staging")
		ensured, err := ensureDir(root)
		if err != nil {
			return "", fmt.Errorf("staging root not writable: %s: %w", root, err)
		}
		return ensured, nil
	}

	root := globalRootDir()
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return ensureDir(root)
}

// BaseDir returns the absolute staging directory for a project/env combination.
func BaseDir(templatePath, composeProject, env string) (string, error) {
	root, err := RootDir(templatePath)
	if err != nil {
		return "", err
	}
	projectKey := ComposeProjectKey(composeProject, env)
	envKey := strings.ToLower(strings.TrimSpace(env))
	if envKey == "" {
		envKey = "default"
	}
	return filepath.Join(root, projectKey, envKey), nil
}

// ConfigDir returns the absolute staging config directory used by runtime code.
func ConfigDir(templatePath, composeProject, env string) (string, error) {
	base, err := BaseDir(templatePath, composeProject, env)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config"), nil
}

func getHostEnv(suffix string) string {
	value, err := envutil.GetHostEnv(suffix)
	if err != nil {
		return ""
	}
	return value
}

func absPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func ensureDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func globalRootDir() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return filepath.Join(xdg, meta.Slug, "staging")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(fmt.Sprintf(".%s", meta.Slug), ".cache", "staging")
	}
	return filepath.Join(home, fmt.Sprintf(".%s", meta.Slug), ".cache", "staging")
}
