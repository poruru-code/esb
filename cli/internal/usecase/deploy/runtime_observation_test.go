package deploy

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestResolveRuntimeObservationFallsBackToRequestValues(t *testing.T) {
	workflow := Workflow{}
	obs, warnings := workflow.resolveRuntimeObservation(Request{
		Context: state.Context{Mode: "docker"},
		Tag:     "latest",
	})
	if obs == nil {
		t.Fatal("expected observation")
	}
	if obs.Mode != "docker" || obs.ESBVersion != "latest" {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestResolveRuntimeObservationUsesContainerImages(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeObservationDockerClient{containers: []container.Summary{
				{
					State: "running",
					Image: "registry:5010/esb-gateway-containerd:v1.2.3",
					Labels: map[string]string{
						compose.ComposeServiceLabel: "gateway",
					},
				},
			}}, nil
		},
	}
	obs, warnings := workflow.resolveRuntimeObservation(Request{
		Context: state.Context{ComposeProject: "esb-dev", Mode: "docker"},
		Tag:     "latest",
	})
	if obs == nil {
		t.Fatal("expected observation")
	}
	if obs.Mode != "containerd" {
		t.Fatalf("expected containerd mode, got %q", obs.Mode)
	}
	if obs.ESBVersion != "v1.2.3" {
		t.Fatalf("expected v1.2.3 tag, got %q", obs.ESBVersion)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestResolveRuntimeObservationQueriesRunningContainersOnly(t *testing.T) {
	var gotAll bool
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeObservationDockerClient{
				containers: nil,
				onListOptions: func(options container.ListOptions) {
					gotAll = options.All
				},
			}, nil
		},
	}

	_, _ = workflow.resolveRuntimeObservation(Request{
		Context: state.Context{ComposeProject: "esb-dev", Mode: "docker"},
		Tag:     "latest",
	})

	if gotAll {
		t.Fatal("expected runtime observation to query running containers only (All=false)")
	}
}

func TestResolveRuntimeObservationIgnoresStoppedContainers(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeObservationDockerClient{containers: []container.Summary{
				{
					State: "exited",
					Image: "registry:5010/esb-gateway-containerd:v1.2.3",
					Labels: map[string]string{
						compose.ComposeServiceLabel: "gateway",
					},
				},
			}}, nil
		},
	}

	obs, warnings := workflow.resolveRuntimeObservation(Request{
		Context: state.Context{ComposeProject: "esb-dev", Mode: "docker"},
		Tag:     "latest",
	})
	if obs == nil {
		t.Fatal("expected observation")
	}
	if obs.Mode != "docker" {
		t.Fatalf("expected fallback mode docker, got %q", obs.Mode)
	}
	if obs.ESBVersion != "latest" {
		t.Fatalf("expected fallback version latest, got %q", obs.ESBVersion)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for missing running compose services")
	}
}

func TestResolveRuntimeObservationWarningsOnDockerClientFailure(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return nil, errors.New("boom")
		},
	}
	obs, warnings := workflow.resolveRuntimeObservation(Request{
		Context: state.Context{ComposeProject: "esb-dev", Mode: "docker"},
		Tag:     "latest",
	})
	if obs == nil {
		t.Fatal("expected fallback observation")
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestApplyArtifactRuntimeConfigUsesRuntimeObservationForRuntimeStack(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions: {}\n")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes: []\n")

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		RuntimeStack: artifactcore.RuntimeStackMeta{
			APIVersion: artifactcore.RuntimeStackAPIVersion,
			Mode:       "docker",
			ESBVersion: "latest",
		},
		Artifacts: []artifactcore.ArtifactEntry{{
			ArtifactRoot:     artifactRoot,
			RuntimeConfigDir: "config",
			SourceTemplate: artifactcore.ArtifactSourceTemplate{
				Path:   "/tmp/template.yaml",
				SHA256: "sha",
			},
		}},
	}
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	workflow := Workflow{}
	if err := workflow.applyArtifactRuntimeConfig(Request{
		ArtifactPath: manifestPath,
		Context:      state.Context{Mode: "docker"},
		Tag:          "latest",
	}, filepath.Join(root, "out")); err != nil {
		t.Fatalf("expected apply to pass: %v", err)
	}
}

func TestApplyArtifactRuntimeConfigAllowsRuntimeStackWhenVersionObservationMissing(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions: {}\n")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes: []\n")

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		RuntimeStack: artifactcore.RuntimeStackMeta{
			APIVersion: artifactcore.RuntimeStackAPIVersion,
			Mode:       "docker",
			ESBVersion: "latest",
		},
		Artifacts: []artifactcore.ArtifactEntry{{
			ArtifactRoot:     artifactRoot,
			RuntimeConfigDir: "config",
			SourceTemplate: artifactcore.ArtifactSourceTemplate{
				Path:   "/tmp/template.yaml",
				SHA256: "sha",
			},
		}},
	}
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	workflow := Workflow{}
	err := workflow.applyArtifactRuntimeConfig(Request{
		ArtifactPath: manifestPath,
		Context:      state.Context{Mode: "docker"},
		Tag:          "",
	}, filepath.Join(root, "out"))
	if err != nil {
		t.Fatalf("expected apply to continue with warning-only runtime compatibility, got: %v", err)
	}
}

type runtimeObservationDockerClient struct {
	containers    []container.Summary
	onListOptions func(container.ListOptions)
}

func (c runtimeObservationDockerClient) ContainerList(_ context.Context, options container.ListOptions) ([]container.Summary, error) {
	if c.onListOptions != nil {
		c.onListOptions(options)
	}
	return c.containers, nil
}

func (c runtimeObservationDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c runtimeObservationDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c runtimeObservationDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c runtimeObservationDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c runtimeObservationDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c runtimeObservationDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c runtimeObservationDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c runtimeObservationDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}
