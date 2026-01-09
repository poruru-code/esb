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
)

type fakeOutputRunner struct {
	outputs map[string]string
}

func (f fakeOutputRunner) Output(_ context.Context, _, _ string, args ...string) ([]byte, error) {
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
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	} {
		if err := os.WriteFile(filepath.Join(rootDir, name), []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	runner := fakeOutputRunner{
		outputs: map[string]string{
			"gateway:443":       "0.0.0.0:10443",
			"runtime-node:443":  "0.0.0.0:20443",
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
	if ports["ESB_PORT_GATEWAY_HTTPS"] != 10443 {
		t.Fatalf("unexpected gateway port: %d", ports["ESB_PORT_GATEWAY_HTTPS"])
	}
	if ports["ESB_PORT_VICTORIALOGS"] != 19428 {
		t.Fatalf("unexpected victorialogs port: %d", ports["ESB_PORT_VICTORIALOGS"])
	}
}
