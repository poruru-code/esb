// Where: cli/internal/wire/wire_test.go
// What: Tests for CLI dependency wiring.
// Why: Ensure BuildDependencies behaves under various init scenarios.
package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
)

type fakeDockerClient struct{}

func (fakeDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return nil, nil
}

func (fakeDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (fakeDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (fakeDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (fakeDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (fakeDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (fakeDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (fakeDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

func TestBuildDependenciesSuccess(t *testing.T) {
	origGetwd := Getwd
	origNewClient := NewDockerClient
	t.Cleanup(func() {
		Getwd = origGetwd
		NewDockerClient = origNewClient
	})

	Getwd = func() (string, error) {
		return "/project", nil
	}
	NewDockerClient = func() (compose.DockerClient, error) {
		return fakeDockerClient{}, nil
	}

	deps, closer, err := BuildDependencies()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deps.ProjectDir != "/project" {
		t.Fatalf("unexpected project dir: %s", deps.ProjectDir)
	}
	if deps.DetectorFactory == nil {
		t.Fatalf("expected detector factory")
	}
	if closer != nil {
		_ = closer.Close()
	}
}

func TestBuildDependenciesGetwdError(t *testing.T) {
	origGetwd := Getwd
	t.Cleanup(func() {
		Getwd = origGetwd
	})

	Getwd = func() (string, error) {
		return "", errors.New("boom")
	}

	_, _, err := BuildDependencies()
	if err == nil {
		t.Fatalf("expected error on getwd failure")
	}
}

func TestBuildDependenciesClientError(t *testing.T) {
	origGetwd := Getwd
	origNewClient := NewDockerClient
	t.Cleanup(func() {
		Getwd = origGetwd
		NewDockerClient = origNewClient
	})

	Getwd = func() (string, error) {
		return "/project", nil
	}
	NewDockerClient = func() (compose.DockerClient, error) {
		return nil, errors.New("client")
	}

	_, _, err := BuildDependencies()
	if err == nil {
		t.Fatalf("expected error on docker client failure")
	}
}
