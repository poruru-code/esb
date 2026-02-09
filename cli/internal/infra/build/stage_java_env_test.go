// Where: cli/internal/infra/build/stage_java_env_test.go
// What: Tests for Java build env/mount argument helpers.
// Why: Keep proxy and Maven settings propagation stable for containerized builds.
package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendJavaBuildEnvArgsMirrorsProxyKeys(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "")
	t.Setenv("no_proxy", "localhost,127.0.0.1")
	t.Setenv("NO_PROXY", "")

	args := appendJavaBuildEnvArgs(nil)
	got := envAssignments(args)

	if got["HTTP_PROXY"] != "http://proxy.example:8080" {
		t.Fatalf("HTTP_PROXY not propagated: %#v", got)
	}
	if got["http_proxy"] != "http://proxy.example:8080" {
		t.Fatalf("http_proxy mirror missing: %#v", got)
	}
	if got["NO_PROXY"] != "localhost,127.0.0.1" {
		t.Fatalf("NO_PROXY mirror missing: %#v", got)
	}
	if got["no_proxy"] != "localhost,127.0.0.1" {
		t.Fatalf("no_proxy not propagated: %#v", got)
	}
}

func TestAppendM2MountArgsFallsBackToSettingsFile(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	m2Dir := filepath.Join(home, ".m2")
	if err := os.MkdirAll(m2Dir, 0o755); err != nil {
		t.Fatalf("mkdir m2: %v", err)
	}
	settingsPath := filepath.Join(m2Dir, "settings.xml")
	if err := os.WriteFile(settingsPath, []byte("<settings/>\n"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := os.Chmod(m2Dir, 0o555); err != nil {
		t.Fatalf("chmod m2: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(m2Dir, 0o755)
	})

	args := appendM2MountArgs(nil, home)
	joined := strings.Join(args, " ")
	want := settingsPath + ":" + hostM2SettingsPath + ":ro"
	if !strings.Contains(joined, want) {
		t.Fatalf("expected fallback settings mount %q in %q", want, joined)
	}
}

func envAssignments(args []string) map[string]string {
	assignments := make(map[string]string)
	for idx := 0; idx < len(args)-1; idx++ {
		if args[idx] != "-e" {
			continue
		}
		key, value, ok := strings.Cut(args[idx+1], "=")
		if !ok {
			continue
		}
		assignments[key] = value
	}
	return assignments
}
