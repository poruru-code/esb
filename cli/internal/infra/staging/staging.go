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
func RootDir() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return filepath.Join(xdg, meta.Slug, "staging")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(fmt.Sprintf(".%s", meta.Slug), ".cache", "staging")
	}
	return filepath.Join(home, fmt.Sprintf(".%s", meta.Slug), ".cache", "staging")
}

// BaseDir returns the absolute staging directory for a project/env combination.
func BaseDir(composeProject, env string) string {
	return filepath.Join(RootDir(), stageKey(composeProject, env))
}

// ConfigDir returns the absolute staging config directory used by runtime code.
func ConfigDir(composeProject, env string) string {
	return filepath.Join(BaseDir(composeProject, env), env, "config")
}
