// Where: cli/internal/app/ports_test.go
// What: Tests for port persistence helpers.
// Why: Ensure discovered ports are stored and applied to env vars.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

func TestSavePortsWritesFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	esbHome := t.TempDir()
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixHome), esbHome)

	ports := map[string]int{
		constants.EnvPortGatewayHTTPS: 10443,
		constants.EnvPortVictoriaLogs: 19428,
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
	if loaded[constants.EnvPortGatewayHTTPS] != 10443 {
		t.Fatalf("unexpected gateway port: %d", loaded[constants.EnvPortGatewayHTTPS])
	}
}

func TestApplyPortsToEnvSetsDerived(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(constants.EnvPortGatewayHTTPS, "")
	t.Setenv(constants.EnvPortVictoriaLogs, "")
	t.Setenv(constants.EnvPortAgentGRPC, "")
	t.Setenv(constants.EnvGatewayPort, "")
	t.Setenv(constants.EnvGatewayURL, "")
	t.Setenv(constants.EnvVictoriaLogsPort, "")
	t.Setenv(constants.EnvVictoriaLogsURL, "")
	t.Setenv(constants.EnvAgentGrpcAddress, "")

	applyPortsToEnv(map[string]int{
		constants.EnvPortGatewayHTTPS: 10443,
		constants.EnvPortVictoriaLogs: 19428,
		constants.EnvPortAgentGRPC:    50051,
	})

	if got := os.Getenv(constants.EnvPortGatewayHTTPS); got != "10443" {
		t.Fatalf("unexpected gateway env: %s", got)
	}
	if got := os.Getenv(constants.EnvGatewayPort); got != "10443" {
		t.Fatalf("unexpected gateway port: %s", got)
	}
	if got := os.Getenv(constants.EnvGatewayURL); got != "https://localhost:10443" {
		t.Fatalf("unexpected gateway url: %s", got)
	}
	if got := os.Getenv(constants.EnvVictoriaLogsURL); got != "http://localhost:19428" {
		t.Fatalf("unexpected victorialogs url: %s", got)
	}
	if got := os.Getenv(constants.EnvAgentGrpcAddress); got != "localhost:50051" {
		t.Fatalf("unexpected agent grpc address: %s", got)
	}
}
