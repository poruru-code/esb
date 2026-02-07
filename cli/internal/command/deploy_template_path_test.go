// Where: cli/internal/command/deploy_template_path_test.go
// What: Tests for template path resolution behavior.
// Why: Ensure relative paths resolve correctly from subdirectories.
package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeTemplatePathResolvesFromRepoRoot(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte(""), 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	tmplDir := filepath.Join(root, "e2e", "fixtures")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	tmplPath := filepath.Join(tmplDir, "template.core.yaml")
	if err := os.WriteFile(tmplPath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	got, err := normalizeTemplatePath("e2e/fixtures/template.core.yaml")
	if err != nil {
		t.Fatalf("normalize template path: %v", err)
	}
	if got != tmplPath {
		t.Fatalf("unexpected template path: %s", got)
	}
}

func TestNormalizeTemplatePathExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmplPath := filepath.Join(home, "template.yaml")
	if err := os.WriteFile(tmplPath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got, err := normalizeTemplatePath("~/template.yaml")
	if err != nil {
		t.Fatalf("normalize template path: %v", err)
	}
	if got != tmplPath {
		t.Fatalf("unexpected template path: %s", got)
	}
}
