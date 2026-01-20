// Where: cli/internal/compose/ports_test.go
// What: Tests for Docker compose port discovery.
// Why: Ensure port parsing and mode filtering behave correctly.
package compose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
)

type fakeOutputRunner struct {
	outputs map[string]string
}

func (f fakeOutputRunner) Run(_ context.Context, _, _ string, _ ...string) error {
	return nil
}

func (f fakeOutputRunner) RunQuiet(_ context.Context, _, _ string, _ ...string) error {
	return nil
}

func (f fakeOutputRunner) RunOutput(_ context.Context, _, _ string, args ...string) ([]byte, error) {
	if len(args) < 4 {
		return nil, errors.New("invalid args")
	}
	service := args[len(args)-2]
	port := args[len(args)-1]
	key := service + ":" + port
	value, ok := f.outputs[key]
	if !ok {
		return nil, errors.New("missing output")
	}
	return []byte(value), nil
}

func TestDiscoverPortsFiltersByMode(t *testing.T) {
	rootDir := t.TempDir()
	for _, name := range []string{
		"docker-compose.docker.yml",
	} {
		if err := os.WriteFile(filepath.Join(rootDir, name), []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	runner := fakeOutputRunner{
		outputs: map[string]string{
			"gateway:8443":      "0.0.0.0:10443",
			"runtime-node:8443": "0.0.0.0:20443",
			"victorialogs:9428": "[::]:19428",
		},
	}

	ports, err := DiscoverPorts(context.Background(), runner, PortDiscoveryOptions{
		RootDir: rootDir,
		Project: "esb-test",
		Mode:    "docker",
		Target:  "control",
	})
	if err != nil {
		t.Fatalf("discover ports: %v", err)
	}
	if ports[constants.EnvPortGatewayHTTPS] != 10443 {
		t.Fatalf("unexpected gateway port: %d", ports[constants.EnvPortGatewayHTTPS])
	}
	if ports[constants.EnvPortVictoriaLogs] != 19428 {
		t.Fatalf("unexpected victorialogs port: %d", ports[constants.EnvPortVictoriaLogs])
	}
}

func TestDiscoverPortsIgnoresZeroPort(t *testing.T) {
	rootDir := t.TempDir()
	// Create all potential compose files that might be looked up
	for _, name := range []string{
		"docker-compose.containerd.yml",
	} {
		if err := os.WriteFile(filepath.Join(rootDir, name), []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	runner := fakeOutputRunner{
		outputs: map[string]string{
			"registry:5010": "0.0.0.0:0",
		},
	}

	ports, err := DiscoverPorts(context.Background(), runner, PortDiscoveryOptions{
		RootDir: rootDir,
		Project: "esb-test",
		Mode:    "containerd",
		Target:  "control",
		Mappings: []PortMapping{
			{
				EnvVar:        constants.EnvPortRegistry,
				Service:       "registry",
				ContainerPort: 5010,
				Modes:         []string{"containerd"},
			},
		},
	})
	if err != nil {
		t.Fatalf("discover ports: %v", err)
	}

	// This is the bug reproduction: currently it accepts 0.
	// We Assert that we DON'T want it to return 0.
	// So if this test fails (returns 0), we know the bug exists.
	// However, to follow TDD, I should write the test expecting the CORRECT behavior (not having the port),
	// and observe it failing if the bug exists.

	if val, ok := ports[constants.EnvPortRegistry]; ok {
		if val == 0 {
			t.Fatalf("expected port 0 to be ignored, but got 0")
		}
	}
}
