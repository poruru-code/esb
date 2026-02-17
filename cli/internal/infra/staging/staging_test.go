// Where: cli/internal/infra/staging/staging_test.go
// What: Unit tests for staging cache path resolution.
// Why: Ensure project cache homes avoid redundant brand segments.
package staging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func TestRootDirUsesRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	marker := filepath.Join(repoRoot, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	chdir(t, repoRoot)

	templateDir := filepath.Join(repoRoot, "nested", "app")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	templatePath := filepath.Join(templateDir, "template.yaml")

	root, err := RootDir(templatePath)
	if err != nil {
		t.Fatalf("root dir: %v", err)
	}
	want := filepath.Join(repoRoot, meta.HomeDir, "staging")
	if filepath.Clean(root) != filepath.Clean(want) {
		t.Fatalf("expected %q, got %q", want, root)
	}
}

func TestRootDirRequiresTemplate(t *testing.T) {
	if _, err := RootDir(""); err == nil {
		t.Fatalf("expected error for empty template path")
	}
}

func TestRootDirRequiresRepoRootMarker(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	templatePath := filepath.Join(tmp, "nested", "template.yaml")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}

	if _, err := RootDir(templatePath); err == nil {
		t.Fatalf("expected error when repo root markers are missing")
	}
}

func TestRootDirUsesCwdWhenTemplateOutsideRepo(t *testing.T) {
	repoRoot := t.TempDir()
	marker := filepath.Join(repoRoot, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	chdir(t, repoRoot)

	external := t.TempDir()
	templatePath := filepath.Join(external, "template.yaml")
	if err := os.WriteFile(templatePath, []byte{}, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	root, err := RootDir(templatePath)
	if err != nil {
		t.Fatalf("root dir: %v", err)
	}
	want := filepath.Join(repoRoot, meta.HomeDir, "staging")
	if filepath.Clean(root) != filepath.Clean(want) {
		t.Fatalf("expected %q, got %q", want, root)
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
