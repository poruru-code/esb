// Where: cli/internal/infra/compose/docker_test.go
// What: Tests for Docker SDK wrappers.
// Why: Ensure container/image checks are scoped and deterministic.
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

type fakeDockerClient struct {
	containers []container.Summary
	images     []image.Summary
	calls      int
}

func (f *fakeDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	f.calls++
	return f.containers, nil
}

func (f *fakeDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	f.calls++
	return f.images, nil
}

func (f *fakeDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	f.calls++
	return nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	f.calls++
	return nil
}

func (f *fakeDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	f.calls++
	return container.PruneReport{}, nil
}

func (f *fakeDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	f.calls++
	return image.PruneReport{}, nil
}

func (f *fakeDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	f.calls++
	return network.PruneReport{}, nil
}

func (f *fakeDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	f.calls++
	return volume.PruneReport{}, nil
}

func TestListContainersByProject(t *testing.T) {
	client := &fakeDockerClient{
		containers: []container.Summary{
			{State: "running", Labels: map[string]string{"com.docker.compose.project": "esb-default"}},
			{State: "exited", Labels: map[string]string{"com.docker.compose.project": "esb-other"}},
			{State: "created", Labels: map[string]string{"other": "value"}},
		},
	}

	containers, err := ListContainersByProject(t.Context(), client, "esb-default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	expected := ContainerInfo{State: "running"}
	if containers[0] != expected {
		t.Fatalf("unexpected container state: %v", containers[0])
	}
}

func TestHasImagesForEnv(t *testing.T) {
	client := &fakeDockerClient{
		images: []image.Summary{
			{RepoTags: []string{"hello:default"}},
			{RepoTags: []string{"other:latest"}},
		},
	}

	hasImages, err := HasImagesForEnv(t.Context(), client, "default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !hasImages {
		t.Fatalf("expected images to be detected")
	}
}

func TestHasImagesForEnv_Missing(t *testing.T) {
	client := &fakeDockerClient{
		images: []image.Summary{
			{RepoTags: []string{"hello:default"}},
		},
	}

	hasImages, err := HasImagesForEnv(t.Context(), client, "staging")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hasImages {
		t.Fatalf("expected no images for env")
	}
}
