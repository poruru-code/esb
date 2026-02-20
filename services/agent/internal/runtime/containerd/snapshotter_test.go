// Where: services/agent/internal/runtime/containerd/snapshotter_test.go
// What: Tests for containerd snapshotter selection logic.
// Why: Ensure firecracker defaults to devmapper while allowing overrides.
package containerd

import (
	"testing"

	"github.com/poruru-code/esb/services/agent/internal/config"
)

func TestResolveSnapshotter_Override(t *testing.T) {
	t.Setenv("CONTAINERD_SNAPSHOTTER", "native")
	t.Setenv("CONTAINERD_RUNTIME", runtimeFirecracker)

	if got := resolveSnapshotter(); got != "native" {
		t.Fatalf("expected override snapshotter 'native', got %q", got)
	}
}

func TestResolveSnapshotter_FirecrackerDefault(t *testing.T) {
	t.Setenv("CONTAINERD_SNAPSHOTTER", "")
	t.Setenv("CONTAINERD_RUNTIME", runtimeFirecracker)

	if got := resolveSnapshotter(); got != config.DefaultSnapshotterDevmapper {
		t.Fatalf("expected firecracker snapshotter %q, got %q", config.DefaultSnapshotterDevmapper, got)
	}
}

func TestResolveSnapshotter_DefaultOverlay(t *testing.T) {
	t.Setenv("CONTAINERD_SNAPSHOTTER", "")
	t.Setenv("CONTAINERD_RUNTIME", "")

	if got := resolveSnapshotter(); got != config.DefaultSnapshotterOverlay {
		t.Fatalf("expected default snapshotter %q, got %q", config.DefaultSnapshotterOverlay, got)
	}
}
