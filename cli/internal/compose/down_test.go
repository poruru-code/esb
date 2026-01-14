// Where: cli/internal/compose/down_test.go
// What: Tests for down operations.
// Why: Ensure containers are stopped/removed by project.
package compose

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
)

type downFakeClient struct {
	containers   []container.Summary
	stopped      []string
	removed      []string
	removeVolume []bool
}

func (f *downFakeClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return f.containers, nil
}

func (f *downFakeClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (f *downFakeClient) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	f.stopped = append(f.stopped, id)
	return nil
}

func (f *downFakeClient) ContainerRemove(_ context.Context, id string, opts container.RemoveOptions) error {
	f.removed = append(f.removed, id)
	f.removeVolume = append(f.removeVolume, opts.RemoveVolumes)
	return nil
}

func (f *downFakeClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (f *downFakeClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (f *downFakeClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (f *downFakeClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

func TestDownProjectStopsAndRemoves(t *testing.T) {
	client := &downFakeClient{
		containers: []container.Summary{
			{ID: "c1", State: "running", Labels: map[string]string{"com.docker.compose.project": "esb-default"}},
			{ID: "c2", State: "exited", Labels: map[string]string{"com.docker.compose.project": "esb-default"}},
			{ID: "c3", State: "running", Labels: map[string]string{"com.docker.compose.project": "esb-other"}},
		},
	}

	if err := DownProject(context.Background(), client, "esb-default", false); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(client.stopped) != 1 || client.stopped[0] != "c1" {
		t.Fatalf("expected to stop c1, got %v", client.stopped)
	}
	if len(client.removed) != 2 {
		t.Fatalf("expected 2 containers removed, got %v", client.removed)
	}
	if len(client.removeVolume) != 2 || client.removeVolume[0] || client.removeVolume[1] {
		t.Fatalf("expected remove volumes false, got %v", client.removeVolume)
	}
}

func TestDownProjectRemoveVolumes(t *testing.T) {
	client := &downFakeClient{
		containers: []container.Summary{
			{ID: "c1", State: "running", Labels: map[string]string{"com.docker.compose.project": "esb-default"}},
		},
	}

	if err := DownProject(context.Background(), client, "esb-default", true); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(client.removeVolume) != 1 || !client.removeVolume[0] {
		t.Fatalf("expected remove volumes true, got %v", client.removeVolume)
	}
}
