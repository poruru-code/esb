// Where: cli/internal/infra/runtime/env_resolver_test.go
// What: Unit tests for runtime environment inference helpers.
// Why: Keep staging and label-based inference deterministic across refactors.
package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func TestInferEnvFromContainerLabels(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "dev" {
			t.Fatalf("expected env 'dev', got %q", got.Env)
		}
		if got.Source != "container label" {
			t.Fatalf("expected source 'container label', got %q", got.Source)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
			{Labels: map[string]string{compose.ESBEnvLabel: "prod"}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})

	t.Run("none", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})
}

func TestInferEnvFromConfigPath(t *testing.T) {
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.docker.yml"), []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}

	templatePath := filepath.Join(tmp, "template.yaml")
	stagingRoot, err := staging.RootDir(templatePath)
	if err != nil {
		t.Fatalf("resolve staging root: %v", err)
	}
	path := filepath.Join(stagingRoot, "esb-dev", "dev", "config")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir staging path: %v", err)
	}

	if got := InferEnvFromConfigPath(path, stagingRoot); got != "dev" {
		t.Fatalf("expected env 'dev', got %q", got)
	}

	if got := InferEnvFromConfigPath(filepath.Join(tmp, "config"), stagingRoot); got != "" {
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

	envs, err := DiscoverStagingEnvs(root, "demo")
	if err != nil {
		t.Fatalf("discover staging envs: %v", err)
	}
	if len(envs) != 2 || envs[0] != "dev" || envs[1] != "prod" {
		t.Fatalf("unexpected envs: %v", envs)
	}
}
