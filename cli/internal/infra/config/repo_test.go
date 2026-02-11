// Where: cli/internal/infra/config/repo_test.go
// What: Tests for repo root resolution.
// Why: Prevent regressions in CWD-based resolution.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRepoRootUsesCwd(t *testing.T) {
	base := t.TempDir()
	repo := makeRepo(t, base, "repo")

	workDir := filepath.Join(repo, "nested")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	chdir(t, workDir)

	root, err := ResolveRepoRoot("")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repo {
		t.Fatalf("expected repo %q, got %q", repo, root)
	}
}

func TestResolveRepoRootErrorsWhenMissing(t *testing.T) {
	startDir := t.TempDir()
	chdir(t, startDir)
	if _, err := ResolveRepoRoot(""); err == nil {
		t.Fatalf("expected error for missing repo root")
	}
}

func TestResolveRepoRootUsesStartDirWhenProvided(t *testing.T) {
	base := t.TempDir()
	repoPath := makeRepo(t, base, "repo-path")

	startDir := filepath.Join(repoPath, "nested")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("create start dir: %v", err)
	}
	chdir(t, t.TempDir())

	root, err := ResolveRepoRoot(startDir)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repoPath {
		t.Fatalf("expected path repo %q, got %q", repoPath, root)
	}
}

func TestResolveRepoRootFromPathIgnoresCwd(t *testing.T) {
	base := t.TempDir()
	repoPath := makeRepo(t, base, "repo-path")

	startDir := filepath.Join(repoPath, "nested")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("create start dir: %v", err)
	}

	chdir(t, t.TempDir())

	root, err := ResolveRepoRootFromPath(startDir)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if root != repoPath {
		t.Fatalf("expected path repo %q, got %q", repoPath, root)
	}
}

func TestResolveRepoRootFromPathErrorsWhenMissing(t *testing.T) {
	startDir := t.TempDir()
	if _, err := ResolveRepoRootFromPath(startDir); err == nil {
		t.Fatalf("expected error for missing repo root")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
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
