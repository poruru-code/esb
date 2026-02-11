// Where: cli/internal/command/deploy_template_discovery_test.go
// What: Tests for template candidate discovery ordering.
// Why: Keep interactive template suggestions deterministic across filesystems.
package command

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverTemplateCandidatesSorted(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"zeta", "alpha", "middle"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(path, "template.yaml"), []byte("Resources: {}\n"), 0o600); err != nil {
			t.Fatalf("write template file: %v", err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	got := discoverTemplateCandidates()
	want := []string{"alpha", "middle", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected candidates: got=%v want=%v", got, want)
	}
}
