// Where: cli/internal/infra/config/repo_test.go
// What: Tests for repo root resolution.
// Why: Prevent regressions in multi-repo resolution priority.
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func setEnvPrefix(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_PREFIX", meta.EnvPrefix)
}

func TestResolveRepoRootUsesEnvFirst(t *testing.T) {
	setEnvPrefix(t)
	base := t.TempDir()
	repoEnv := makeRepo(t, base, "repo-env")
	repoStart := makeRepo(t, base, "repo-start")
	t.Setenv("ESB_REPO", repoEnv)

	startDir := filepath.Join(repoStart, "nested")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("create start dir: %v", err)
	}

	root, err := ResolveRepoRoot(startDir)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repoEnv {
		t.Fatalf("expected env repo %q, got %q", repoEnv, root)
	}
}

func TestResolveRepoRootUsesStartDirWhenEnvEmpty(t *testing.T) {
	setEnvPrefix(t)
	base := t.TempDir()
	repoStart := makeRepo(t, base, "repo-start")
	t.Setenv("ESB_REPO", "")

	startDir := filepath.Join(repoStart, "nested")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("create start dir: %v", err)
	}

	root, err := ResolveRepoRoot(startDir)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repoStart {
		t.Fatalf("expected start repo %q, got %q", repoStart, root)
	}
}

func TestResolveRepoRootErrorsWhenMissing(t *testing.T) {
	setEnvPrefix(t)
	t.Setenv("ESB_REPO", "")

	startDir := t.TempDir()
	if _, err := ResolveRepoRoot(startDir); err == nil {
		t.Fatalf("expected error for missing repo root")
	}
}

func TestResolveRepoRootFromPathIgnoresEnvAndGlobal(t *testing.T) {
	setEnvPrefix(t)
	base := t.TempDir()
	repoEnv := makeRepo(t, base, "repo-env")
	repoPath := makeRepo(t, base, "repo-path")

	t.Setenv("ESB_REPO", repoEnv)

	startDir := filepath.Join(repoPath, "nested")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("create start dir: %v", err)
	}

	root, err := ResolveRepoRootFromPath(startDir)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repoPath {
		t.Fatalf("expected path repo %q, got %q", repoPath, root)
	}
}

func TestResolveRepoRootFromPathErrorsWhenMissing(t *testing.T) {
	setEnvPrefix(t)
	startDir := t.TempDir()
	if _, err := ResolveRepoRootFromPath(startDir); err == nil {
		t.Fatalf("expected error for missing repo root")
	}
}

func makeRepo(t *testing.T, base, name string) string {
	t.Helper()
	repo := filepath.Join(base, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	marker := filepath.Join(repo, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte(""), 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	return repo
}
