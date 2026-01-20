// Where: cli/internal/helpers/state_store_test.go
// What: Tests for the port state store implementation.
// Why: Ensure discovered ports are persisted and removable as expected.
package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestPortStateStoreSaveLoadRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	esbHome := t.TempDir()
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixHome), esbHome)

	store := NewPortStateStore()
	ctx := state.Context{Env: "staging"}
	ports := map[string]int{
		constants.EnvPortGatewayHTTPS: 10443,
		constants.EnvPortVictoriaLogs: 19428,
	}

	if err := store.Save(ctx, ports); err != nil {
		t.Fatalf("save ports: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load ports: %v", err)
	}
	if loaded[constants.EnvPortGatewayHTTPS] != 10443 {
		t.Fatalf("unexpected gateway port: %d", loaded[constants.EnvPortGatewayHTTPS])
	}

	if err := store.Remove(ctx); err != nil {
		t.Fatalf("remove ports: %v", err)
	}

	path := filepath.Join(esbHome, "ports.json")
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("ports file still exists: %v", err)
	}
}
