// Where: cli/internal/command/deploy_runtime_env_test.go
// What: Unit tests for deploy runtime env inference env.
// Why: Ensure env inference behaves deterministically for staging paths.
package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func TestInferEnvFromConfigPath(t *testing.T) {
	tmp := t.TempDir()

	templatePath := filepath.Join(tmp, "template.yaml")
	stagingRoot, err := staging.RootDir(templatePath)
	if err != nil {
		t.Fatalf("resolve staging root: %v", err)
	}
	path := filepath.Join(stagingRoot, "esb-dev", "dev", "config")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir staging path: %v", err)
	}

	if got := inferEnvFromConfigPath(path, stagingRoot); got != "dev" {
		t.Fatalf("expected env 'dev', got %q", got)
	}

	if got := inferEnvFromConfigPath(filepath.Join(tmp, "config"), stagingRoot); got != "" {
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

	mk(filepath.Join(root, "demo", "dev", "config"))
	mk(filepath.Join(root, "demo", "prod", "config"))
	mk(filepath.Join(root, "other", "staging", "config"))

	envs, err := discoverStagingEnvs(root, "demo")
	if err != nil {
		t.Fatalf("discover staging envs: %v", err)
	}
	if len(envs) != 2 || envs[0] != "dev" || envs[1] != "prod" {
		t.Fatalf("unexpected envs: %v", envs)
	}
}
