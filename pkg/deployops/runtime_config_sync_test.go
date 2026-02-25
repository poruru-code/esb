package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncRuntimeConfigReplacesBindTargetContents(t *testing.T) {
	stagingDir := t.TempDir()
	runtimeDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(runtimeDir, "stale.yml"), []byte("stale: true\n"), 0o600); err != nil {
		t.Fatalf("write stale runtime file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(stagingDir, "nested"), 0o755); err != nil {
		t.Fatalf("create nested staging directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stagingDir, "functions.yml"),
		[]byte("functions: {}\n"),
		0o600,
	); err != nil {
		t.Fatalf("write functions.yml: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stagingDir, "nested", "routing.yml"),
		[]byte("routes: []\n"),
		0o600,
	); err != nil {
		t.Fatalf("write nested routing.yml: %v", err)
	}

	if err := syncRuntimeConfig(stagingDir, RuntimeConfigTarget{BindPath: runtimeDir}); err != nil {
		t.Fatalf("syncRuntimeConfig() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(runtimeDir, "stale.yml")); !os.IsNotExist(err) {
		t.Fatalf("stale file was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "functions.yml")); err != nil {
		t.Fatalf("functions.yml was not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "nested", "routing.yml")); err != nil {
		t.Fatalf("nested routing.yml was not copied: %v", err)
	}
}

func TestSyncRuntimeConfigVolumeUsesDockerRun(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("functions: {}\n"), 0o600); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	original := runDockerForSync
	t.Cleanup(func() { runDockerForSync = original })

	var captured []string
	runDockerForSync = func(args ...string) ([]byte, error) {
		captured = append([]string(nil), args...)
		return nil, nil
	}

	if err := syncRuntimeConfig(stagingDir, RuntimeConfigTarget{VolumeName: "esb-runtime-config"}); err != nil {
		t.Fatalf("syncRuntimeConfig() error = %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "run --rm") {
		t.Fatalf("expected docker run invocation, got %q", joined)
	}
	if !strings.Contains(joined, "-v esb-runtime-config:/runtime-config") {
		t.Fatalf("expected runtime-config volume mount, got %q", joined)
	}
}

func TestSyncRuntimeConfigRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is platform/permission dependent on windows")
	}
	stagingDir := t.TempDir()
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("functions: {}\n"), 0o600); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	if err := os.Symlink(filepath.Join(stagingDir, "functions.yml"), filepath.Join(stagingDir, "link.yml")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if err := syncRuntimeConfig(stagingDir, RuntimeConfigTarget{BindPath: runtimeDir}); err == nil {
		t.Fatal("syncRuntimeConfig() expected symlink error")
	}
}

func TestSyncRuntimeConfigRequiresTarget(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("functions: {}\n"), 0o600); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	err := syncRuntimeConfig(stagingDir, RuntimeConfigTarget{})
	if !errors.Is(err, errRuntimeConfigTargetRequired) {
		t.Fatalf("expected errRuntimeConfigTargetRequired, got %v", err)
	}
}
