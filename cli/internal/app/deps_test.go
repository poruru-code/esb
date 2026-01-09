// Where: cli/internal/app/deps_test.go
// What: Tests for dependency wiring.
// Why: Ensure detector factory is configured predictably.
package app

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
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

func TestNewDetectorFactory(t *testing.T) {
	factory := NewDetectorFactory(fakeDockerClient{}, nil)
	detector, err := factory("/project", "default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	det, ok := detector.(state.Detector)
	if !ok {
		t.Fatalf("expected state.Detector type")
	}
	if det.ProjectDir != "/project" {
		t.Fatalf("unexpected project dir: %s", det.ProjectDir)
	}
	if det.Env != "default" {
		t.Fatalf("unexpected env: %s", det.Env)
	}
	if det.ResolveContext == nil || det.ListContainers == nil || det.HasBuildArtifacts == nil {
		t.Fatalf("expected detector dependencies to be configured")
	}
}

func TestNewDetectorFactory_NilClient(t *testing.T) {
	factory := NewDetectorFactory(nil, nil)
	_, err := factory("/project", "default")
	if err == nil {
		t.Fatalf("expected error when client is nil")
	}
	if !errors.Is(err, ErrDockerClientNil) {
		t.Fatalf("expected ErrDockerClientNil, got %v", err)
	}
}
