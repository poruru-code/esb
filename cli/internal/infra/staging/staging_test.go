// Where: cli/internal/infra/staging/staging_test.go
// What: Unit tests for staging cache path resolution.
// Why: Ensure project cache homes avoid redundant brand segments.
package staging

import (
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestRootDirUsesTemplateDir(t *testing.T) {
	tmp := t.TempDir()
	templatePath := filepath.Join(tmp, "template.yaml")

	root, err := RootDir(templatePath)
	if err != nil {
		t.Fatalf("root dir: %v", err)
	}
	want := filepath.Join(tmp, meta.OutputDir, "staging")
	if filepath.Clean(root) != filepath.Clean(want) {
		t.Fatalf("expected %q, got %q", want, root)
	}
}

func TestRootDirRequiresTemplate(t *testing.T) {
	if _, err := RootDir(""); err == nil {
		t.Fatalf("expected error for empty template path")
	}
}
