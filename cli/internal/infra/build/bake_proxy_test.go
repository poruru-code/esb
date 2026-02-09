// Where: cli/internal/infra/build/bake_proxy_test.go
// What: Tests for buildx proxy driver environment propagation.
// Why: Ensure NO_PROXY lists are preserved when configuring buildkit containers.
package build

import (
	"strings"
	"testing"
)

func TestBuildxProxyDriverEnvMapKeepsNoProxyList(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,registry")
	t.Setenv("no_proxy", "")

	driverEnv := buildxProxyDriverEnvMap()
	want := "localhost,127.0.0.1,registry"
	if got := driverEnv["NO_PROXY"]; got != want {
		t.Fatalf("NO_PROXY=%q, want %q", got, want)
	}
	if got := driverEnv["no_proxy"]; got != want {
		t.Fatalf("no_proxy=%q, want %q", got, want)
	}
}

func TestBuildxProxyDriverOptsFromMapIncludesNoProxy(t *testing.T) {
	opts := buildxProxyDriverOptsFromMap(map[string]string{
		"NO_PROXY": "localhost,127.0.0.1,registry",
	})
	joined := strings.Join(opts, " ")
	if !strings.Contains(joined, "env.NO_PROXY=localhost,127.0.0.1,registry") {
		t.Fatalf("NO_PROXY option missing from driver opts: %v", opts)
	}
}
