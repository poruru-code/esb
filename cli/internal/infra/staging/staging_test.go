// Where: cli/internal/infra/staging/staging_test.go
// What: Unit tests for staging cache path resolution.
// Why: Ensure project cache homes avoid redundant brand segments.
package staging

import (
	"os"
	"path/filepath"
	"strings"
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

func TestComposeProjectKey(t *testing.T) {
	if got := ComposeProjectKey("custom-project", "dev"); got != "custom-project" {
		t.Fatalf("ComposeProjectKey() = %q, want %q", got, "custom-project")
	}
	if got := ComposeProjectKey("", "Dev"); got != meta.Slug+"-dev" {
		t.Fatalf("ComposeProjectKey() env fallback = %q", got)
	}
	if got := ComposeProjectKey("", ""); got != meta.Slug {
		t.Fatalf("ComposeProjectKey() default = %q, want %q", got, meta.Slug)
	}
}

func TestCacheKeyStableAndEnvSensitive(t *testing.T) {
	a := CacheKey("esb-dev", "dev")
	b := CacheKey("esb-dev", "dev")
	c := CacheKey("esb-dev", "staging")
	if a != b {
		t.Fatalf("CacheKey must be stable: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("CacheKey must vary by env: %q", a)
	}
	if !strings.HasPrefix(a, "esb-dev-") {
		t.Fatalf("CacheKey prefix mismatch: %q", a)
	}
}

func TestBaseDirAndConfigDir(t *testing.T) {
	repoRoot := t.TempDir()
	marker := filepath.Join(repoRoot, "docker-compose.docker.yml")
	if err := os.WriteFile(marker, []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	chdir(t, repoRoot)

	templatePath := filepath.Join(repoRoot, "template.yaml")
	if err := os.WriteFile(templatePath, []byte{}, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	base, err := BaseDir(templatePath, "esb-dev", "dev")
	if err != nil {
		t.Fatalf("BaseDir() error = %v", err)
	}
	wantBase := filepath.Join(repoRoot, meta.HomeDir, "staging", "esb-dev", "dev")
	if filepath.Clean(base) != filepath.Clean(wantBase) {
		t.Fatalf("BaseDir() = %q, want %q", base, wantBase)
	}

	configDir, err := ConfigDir(templatePath, "esb-dev", "dev")
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	wantConfig := filepath.Join(wantBase, "config")
	if filepath.Clean(configDir) != filepath.Clean(wantConfig) {
		t.Fatalf("ConfigDir() = %q, want %q", configDir, wantConfig)
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
