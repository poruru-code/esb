// Where: cli/internal/app/ports_test.go
// What: Tests for port persistence helpers.
// Why: Ensure discovered ports are stored and applied to env vars.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSavePortsWritesFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	esbHome := t.TempDir()
	t.Setenv("ESB_HOME", esbHome)

	ports := map[string]int{
		"ESB_PORT_GATEWAY_HTTPS": 10443,
		"ESB_PORT_VICTORIALOGS":  19428,
	}

	path, err := savePorts("staging", ports)
	if err != nil {
		t.Fatalf("save ports: %v", err)
	}
	expectedPath := filepath.Join(esbHome, "ports.json")
	if path != expectedPath {
		t.Fatalf("unexpected ports path: %s", path)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ports file: %v", err)
	}
	var loaded map[string]int
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatalf("unmarshal ports: %v", err)
	}
	if loaded["ESB_PORT_GATEWAY_HTTPS"] != 10443 {
		t.Fatalf("unexpected gateway port: %d", loaded["ESB_PORT_GATEWAY_HTTPS"])
	}
}

func TestApplyPortsToEnvSetsDerived(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ESB_PORT_GATEWAY_HTTPS", "")
	t.Setenv("ESB_PORT_VICTORIALOGS", "")
	t.Setenv("ESB_PORT_AGENT_GRPC", "")
	t.Setenv("GATEWAY_PORT", "")
	t.Setenv("GATEWAY_URL", "")
	t.Setenv("VICTORIALOGS_PORT", "")
	t.Setenv("VICTORIALOGS_URL", "")
	t.Setenv("VICTORIALOGS_QUERY_URL", "")
	t.Setenv("AGENT_GRPC_ADDRESS", "")

	applyPortsToEnv(map[string]int{
		"ESB_PORT_GATEWAY_HTTPS": 10443,
		"ESB_PORT_VICTORIALOGS":  19428,
		"ESB_PORT_AGENT_GRPC":    50051,
	})

	if got := os.Getenv("ESB_PORT_GATEWAY_HTTPS"); got != "10443" {
		t.Fatalf("unexpected gateway env: %s", got)
	}
	if got := os.Getenv("GATEWAY_PORT"); got != "10443" {
		t.Fatalf("unexpected gateway port: %s", got)
	}
	if got := os.Getenv("GATEWAY_URL"); got != "https://localhost:10443" {
		t.Fatalf("unexpected gateway url: %s", got)
	}
	if got := os.Getenv("VICTORIALOGS_URL"); got != "http://localhost:19428" {
		t.Fatalf("unexpected victorialogs url: %s", got)
	}
	if got := os.Getenv("AGENT_GRPC_ADDRESS"); got != "localhost:50051" {
		t.Fatalf("unexpected agent grpc address: %s", got)
	}
}
