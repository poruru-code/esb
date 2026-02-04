// Where: cli/internal/infra/staging/staging_test.go
// What: Unit tests for staging cache path resolution.
// Why: Ensure project cache homes avoid redundant brand segments.
package staging

import (
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestRootDirProjectCacheHomeNoBrand(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_STAGING_DIR", "")
	t.Setenv("ESB_STAGING_HOME", "")

	tmp := t.TempDir()
	cacheHome := filepath.Join(tmp, meta.OutputDir, ".cache")
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	root, err := RootDir("")
	if err != nil {
		t.Fatalf("root dir: %v", err)
	}
	want := filepath.Join(cacheHome, "staging")
	if filepath.Clean(root) != filepath.Clean(want) {
		t.Fatalf("expected %q, got %q", want, root)
	}
}

func TestRootDirXDGKeepsBrandNamespace(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_STAGING_DIR", "")
	t.Setenv("ESB_STAGING_HOME", "")

	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	root, err := RootDir("")
	if err != nil {
		t.Fatalf("root dir: %v", err)
	}
	want := filepath.Join(tmp, meta.Slug, "staging")
	if filepath.Clean(root) != filepath.Clean(want) {
		t.Fatalf("expected %q, got %q", want, root)
	}
}
