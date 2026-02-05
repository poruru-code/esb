// Where: cli/internal/infra/staging/staging_test.go
// What: Unit tests for staging cache path resolution.
// Why: Ensure project cache homes avoid redundant brand segments.
package staging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestRootDirUsesRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	marker := filepath.Join(repoRoot, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}

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
	templatePath := filepath.Join(tmp, "nested", "template.yaml")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}

	if _, err := RootDir(templatePath); err == nil {
		t.Fatalf("expected error when repo root markers are missing")
	}
}
