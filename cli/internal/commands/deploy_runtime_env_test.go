// Where: cli/internal/commands/deploy_runtime_env_test.go
// What: Unit tests for deploy runtime env inference helpers.
// Why: Ensure env inference behaves deterministically for staging paths.
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

func TestInferEnvFromConfigPath(t *testing.T) {
	tmp := t.TempDir()
	old := os.Getenv("XDG_CACHE_HOME")
	t.Setenv("XDG_CACHE_HOME", tmp)
	t.Cleanup(func() { _ = os.Setenv("XDG_CACHE_HOME", old) })

	stagingRoot := staging.RootDir()
	path := filepath.Join(stagingRoot, "esb-dev-aaaa", "dev", "config")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir staging path: %v", err)
	}

	if got := inferEnvFromConfigPath(path); got != "dev" {
		t.Fatalf("expected env 'dev', got %q", got)
	}

	if got := inferEnvFromConfigPath(filepath.Join(tmp, "config")); got != "" {
		t.Fatalf("expected empty env for non-staging path, got %q", got)
	}
}

func TestDiscoverStagingEnvs(t *testing.T) {
	root := t.TempDir()
	mk := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	mk(filepath.Join(root, "demo-aaaa", "dev", "config"))
	mk(filepath.Join(root, "demo-aaaa", "prod", "config"))
	mk(filepath.Join(root, "other-bbbb", "staging", "config"))

	envs, err := discoverStagingEnvs(root, "demo")
	if err != nil {
		t.Fatalf("discover staging envs: %v", err)
	}
	if len(envs) != 2 || envs[0] != "dev" || envs[1] != "prod" {
		t.Fatalf("unexpected envs: %v", envs)
	}
}
