// Where: cli/internal/infra/runtime/env_resolver_test.go
// What: Unit tests for runtime environment inference helpers.
// Why: Keep staging and label-based inference deterministic across refactors.
package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
	"github.com/poruru-code/esb/cli/internal/infra/staging"
)

func TestInferEnvFromContainerLabels(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "dev" {
			t.Fatalf("expected env 'dev', got %q", got.Env)
		}
		if got.Source != "container label" {
			t.Fatalf("expected source 'container label', got %q", got.Source)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
			{Labels: map[string]string{compose.ESBEnvLabel: "prod"}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})

	t.Run("none", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{}},
		}
		got := InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})
}

func TestInferEnvFromConfigPath(t *testing.T) {
	tmp := t.TempDir()
	setWorkingDir(t, tmp)

	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.docker.yml"), []byte{}, 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}

	templatePath := filepath.Join(tmp, "template.yaml")
	stagingRoot, err := staging.RootDir(templatePath)
	if err != nil {
		t.Fatalf("resolve staging root: %v", err)
	}
	path := filepath.Join(stagingRoot, "esb-dev", "dev", "config")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir staging path: %v", err)
	}

	if got := InferEnvFromConfigPath(path, stagingRoot); got != "dev" {
		t.Fatalf("expected env 'dev', got %q", got)
	}

	if got := InferEnvFromConfigPath(filepath.Join(tmp, "config"), stagingRoot); got != "" {
		t.Fatalf("expected empty env for non-staging path, got %q", got)
	}
}

func TestDiscoverStagingEnvs(t *testing.T) {
	root := t.TempDir()
	mk := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	mk(filepath.Join(root, "demo", "dev", "config"))
	mk(filepath.Join(root, "demo", "prod", "config"))
	mk(filepath.Join(root, "other", "staging", "config"))

	envs, err := DiscoverStagingEnvs(root, "demo")
	if err != nil {
		t.Fatalf("discover staging envs: %v", err)
	}
	if len(envs) != 2 || envs[0] != "dev" || envs[1] != "prod" {
		t.Fatalf("unexpected envs: %v", envs)
	}
}

func TestSelectGatewayContainerDeterministic(t *testing.T) {
	selected := selectGatewayContainer([]container.Summary{
		{
			ID:    "exited-zeta",
			State: "exited",
			Names: []string{"/z-gateway"},
		},
		{
			ID:    "running-zeta",
			State: "running",
			Names: []string{"/z-gateway"},
		},
		{
			ID:    "running-alpha",
			State: "running",
			Names: []string{"/a-gateway"},
		},
	})
	if selected.ID != "running-alpha" {
		t.Fatalf("expected running-alpha, got %q", selected.ID)
	}
}

func TestDockerEnvResolverRequiresFactory(t *testing.T) {
	resolver := DockerEnvResolver{}
	_, err := resolver.InferEnvFromProject("esb-dev", "")
	if err == nil {
		t.Fatal("expected docker client factory configuration error")
	}
	if !strings.Contains(err.Error(), "docker client factory is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerEnvResolverReturnsFactoryError(t *testing.T) {
	resolver := DockerEnvResolver{
		DockerClientFactory: func() (compose.DockerClient, error) {
			return nil, errors.New("boom")
		},
	}
	_, err := resolver.InferEnvFromProject("esb-dev", "")
	if err == nil {
		t.Fatal("expected factory error")
	}
	if !strings.Contains(err.Error(), "create docker client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerEnvResolverRejectsNilClient(t *testing.T) {
	resolver := DockerEnvResolver{
		DockerClientFactory: func() (compose.DockerClient, error) {
			return nil, nil
		},
	}
	_, err := resolver.InferEnvFromProject("esb-dev", "")
	if err == nil {
		t.Fatal("expected nil client error")
	}
	if !strings.Contains(err.Error(), "returned nil client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerEnvResolverInferEnvFromRunningLabels(t *testing.T) {
	resolver := DockerEnvResolver{
		DockerClientFactory: func() (compose.DockerClient, error) {
			return envResolverDockerClient{
				containers: []container.Summary{
					{
						Labels: map[string]string{
							compose.ESBEnvLabel: "dev",
						},
					},
				},
			}, nil
		},
	}
	got, err := resolver.InferEnvFromProject("esb-dev", "template.yaml")
	if err != nil {
		t.Fatalf("infer env: %v", err)
	}
	if got.Env != "dev" {
		t.Fatalf("expected dev, got %q", got.Env)
	}
	if got.Source != "container label" {
		t.Fatalf("expected source container label, got %q", got.Source)
	}
}

type envResolverDockerClient struct {
	containers []container.Summary
}

func (c envResolverDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return c.containers, nil
}

func (c envResolverDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c envResolverDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c envResolverDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c envResolverDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c envResolverDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c envResolverDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c envResolverDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c envResolverDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

func setWorkingDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd %s: %v", wd, err)
		}
	})
}
