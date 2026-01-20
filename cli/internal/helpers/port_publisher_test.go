// Where: cli/internal/helpers/port_publisher_test.go
// What: Tests for port publisher persistence behavior.
// Why: Verify change detection and persistence wiring.
package helpers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type fakePortDiscoverer struct {
	ports map[string]int
	err   error
}

func (f fakePortDiscoverer) Discover(_ context.Context, _, _, _ string) (map[string]int, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ports, nil
}

type stubStateStore struct {
	load  map[string]int
	saved map[string]int
}

func (s *stubStateStore) Load(_ state.Context) (map[string]int, error) {
	return s.load, nil
}

func (s *stubStateStore) Save(_ state.Context, ports map[string]int) error {
	s.saved = clonePorts(ports)
	return nil
}

func (s *stubStateStore) Remove(_ state.Context) error {
	return nil
}

func TestDiscoverAndPersistPortsChanged(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.yml"), []byte(""), 0o644); err != nil {
		t.Fatalf("write docker-compose fixture: %v", err)
	}
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixRepo), repoRoot)

	ctx := state.Context{
		ProjectDir:     t.TempDir(),
		ComposeProject: "esb-dev",
		Env:            "dev",
	}
	discoverer := fakePortDiscoverer{
		ports: map[string]int{constants.EnvPortGatewayHTTPS: 10001},
	}
	store := &stubStateStore{
		load: map[string]int{constants.EnvPortGatewayHTTPS: 10000},
	}

	result, err := DiscoverAndPersistPorts(ctx, discoverer, store)
	if err != nil {
		t.Fatalf("DiscoverAndPersistPorts: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed=true")
	}
	if !reflect.DeepEqual(store.saved, discoverer.ports) {
		t.Fatalf("expected ports to be saved")
	}
	if !reflect.DeepEqual(result.Published, discoverer.ports) {
		t.Fatalf("expected published ports to match discovered")
	}
}

func TestDiscoverAndPersistPortsUnchanged(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.yml"), []byte(""), 0o644); err != nil {
		t.Fatalf("write docker-compose fixture: %v", err)
	}
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixRepo), repoRoot)

	ctx := state.Context{
		ProjectDir:     t.TempDir(),
		ComposeProject: "esb-dev",
		Env:            "dev",
	}
	ports := map[string]int{constants.EnvPortGatewayHTTPS: 10001}
	discoverer := fakePortDiscoverer{ports: ports}
	store := &stubStateStore{load: clonePorts(ports)}

	result, err := DiscoverAndPersistPorts(ctx, discoverer, store)
	if err != nil {
		t.Fatalf("DiscoverAndPersistPorts: %v", err)
	}
	if result.Changed {
		t.Fatalf("expected changed=false")
	}
	if !reflect.DeepEqual(result.Published, ports) {
		t.Fatalf("expected published ports to match discovered")
	}
}

func clonePorts(ports map[string]int) map[string]int {
	if ports == nil {
		return nil
	}
	clone := make(map[string]int, len(ports))
	for key, value := range ports {
		clone[key] = value
	}
	return clone
}
