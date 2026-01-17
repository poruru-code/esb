// Where: cli/internal/compose/prune_test.go
// What: Tests for project-scoped prune helpers.
// Why: Ensure prune filters match project labels and flags.
package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
)

type pruneFakeClient struct {
	containerFilters []filters.Args
	networkFilters   []filters.Args
	volumeFilters    []filters.Args
	imageFilters     []filters.Args
}

func (f *pruneFakeClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return nil, nil
}

func (f *pruneFakeClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (f *pruneFakeClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (f *pruneFakeClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (f *pruneFakeClient) ContainersPrune(_ context.Context, pruneFilters filters.Args) (container.PruneReport, error) {
	f.containerFilters = append(f.containerFilters, pruneFilters)
	return container.PruneReport{ContainersDeleted: []string{"c1"}, SpaceReclaimed: 10}, nil
}

func (f *pruneFakeClient) ImagesPrune(_ context.Context, pruneFilters filters.Args) (image.PruneReport, error) {
	f.imageFilters = append(f.imageFilters, pruneFilters)
	return image.PruneReport{ImagesDeleted: []image.DeleteResponse{{Deleted: "i1"}}, SpaceReclaimed: 5}, nil
}

func (f *pruneFakeClient) NetworksPrune(_ context.Context, pruneFilters filters.Args) (network.PruneReport, error) {
	f.networkFilters = append(f.networkFilters, pruneFilters)
	return network.PruneReport{NetworksDeleted: []string{"n1"}}, nil
}

func (f *pruneFakeClient) VolumesPrune(_ context.Context, pruneFilters filters.Args) (volume.PruneReport, error) {
	f.volumeFilters = append(f.volumeFilters, pruneFilters)
	return volume.PruneReport{VolumesDeleted: []string{"v1"}, SpaceReclaimed: 7}, nil
}

func TestPruneProjectDefaults(t *testing.T) {
	client := &pruneFakeClient{}
	report, err := PruneProject(context.Background(), client, PruneOptions{Project: "demo-default"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(client.containerFilters) != 1 {
		t.Fatalf("expected container prune called once")
	}
	if len(client.networkFilters) != 1 {
		t.Fatalf("expected network prune called once")
	}
	if len(client.volumeFilters) != 0 {
		t.Fatalf("expected volumes prune skipped")
	}
	if len(client.imageFilters) != 2 {
		t.Fatalf("expected image prune called twice, got %d", len(client.imageFilters))
	}
	if !hasLabel(client.containerFilters[0], ComposeProjectLabel, "demo-default") {
		t.Fatalf("missing compose project label for containers")
	}
	if !hasLabel(client.networkFilters[0], ComposeProjectLabel, "demo-default") {
		t.Fatalf("missing compose project label for networks")
	}
	for _, f := range client.imageFilters {
		if dangling := getFilterValue(f, "dangling"); dangling != "true" {
			t.Fatalf("expected dangling=true, got %q", dangling)
		}
	}
	if got := report.SpaceReclaimed; got != 20 {
		t.Fatalf("unexpected reclaimed space: %d", got)
	}
}

func TestPruneProjectWithVolumesAndAllImages(t *testing.T) {
	client := &pruneFakeClient{}
	report, err := PruneProject(context.Background(), client, PruneOptions{
		Project:       "demo-default",
		RemoveVolumes: true,
		AllImages:     true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(client.volumeFilters) != 1 {
		t.Fatalf("expected volumes prune called once")
	}
	for _, f := range client.imageFilters {
		if dangling := getFilterValue(f, "dangling"); dangling != "false" {
			t.Fatalf("expected dangling=false, got %q", dangling)
		}
	}
	if got := report.SpaceReclaimed; got != 27 {
		t.Fatalf("unexpected reclaimed space: %d", got)
	}
}

func hasLabel(pruneFilters filters.Args, key, value string) bool {
	for _, label := range pruneFilters.Get("label") {
		if label == fmt.Sprintf("%s=%s", key, value) {
			return true
		}
	}
	return false
}

func getFilterValue(pruneFilters filters.Args, key string) string {
	values := pruneFilters.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
