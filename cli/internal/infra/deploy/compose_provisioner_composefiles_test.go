// Where: cli/internal/infra/deploy/compose_provisioner_composefiles_test.go
// What: Tests for compose file resolution in compose provisioner.
// Why: Ensure Docker client creation is injectable and error paths are explicit.
package deploy

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
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

type provisionerDockerClient struct {
	containers []container.Summary
	listErr    error
}

type provisionerRunner struct {
	serviceChecks int
	helpChecks    int
	runCalls      int
	runQuietCalls int
	runOutputFn   func(args []string) ([]byte, error)
	runErr        error
	runQuietErr   error
	runQuietArgs  [][]string
}

func (c *provisionerDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.containers, nil
}

func (c *provisionerDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c *provisionerDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c *provisionerDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c *provisionerDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c *provisionerDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c *provisionerDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c *provisionerDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c *provisionerDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

func (r *provisionerRunner) Run(_ context.Context, _, _ string, _ ...string) error {
	r.runCalls++
	if r.runErr != nil {
		return r.runErr
	}
	return nil
}

func (r *provisionerRunner) RunOutput(_ context.Context, _, _ string, args ...string) ([]byte, error) {
	if r.runOutputFn != nil {
		return r.runOutputFn(args)
	}
	if containsArg(args, "--help") {
		r.helpChecks++
		return []byte("--no-warn-orphans"), nil
	}
	if containsArg(args, "config") && containsArg(args, "--services") {
		r.serviceChecks++
		return []byte("provisioner\ndatabase\ns3-storage\nvictorialogs\n"), nil
	}
	return []byte{}, nil
}

func (r *provisionerRunner) RunQuiet(_ context.Context, _, _ string, args ...string) error {
	r.runQuietCalls++
	r.runQuietArgs = append(r.runQuietArgs, append([]string(nil), args...))
	if r.runQuietErr != nil {
		return r.runQuietErr
	}
	return nil
}

func TestResolveComposeFilesForProjectRequiresDockerFactory(t *testing.T) {
	p := newComposeProvisioner(nil, nil, nil)
	_, err := p.resolveComposeFilesForProject(t.Context(), "esb-dev")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker client factory is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveComposeFilesForProjectReturnsFactoryError(t *testing.T) {
	p := newComposeProvisioner(nil, nil, func() (compose.DockerClient, error) {
		return nil, errors.New("boom")
	})
	_, err := p.resolveComposeFilesForProject(t.Context(), "esb-dev")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create docker client: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveComposeFilesForProjectRejectsNilClient(t *testing.T) {
	p := newComposeProvisioner(nil, nil, func() (compose.DockerClient, error) {
		return nil, nil
	})
	_, err := p.resolveComposeFilesForProject(t.Context(), "esb-dev")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker client factory returned nil client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveComposeFilesForProjectReturnsComposeFiles(t *testing.T) {
	root := t.TempDir()
	configFile := filepath.Join(root, "docker-compose.yml")
	if err := writeTestComposeFile(configFile, "services: {}"); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	client := &provisionerDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					compose.ComposeProjectLabel:     "esb-dev",
					compose.ComposeConfigFilesLabel: "docker-compose.yml",
					compose.ComposeWorkingDirLabel:  root,
				},
			},
		},
	}
	p := newComposeProvisioner(nil, nil, func() (compose.DockerClient, error) {
		return client, nil
	})

	result, err := p.resolveComposeFilesForProject(context.Background(), "esb-dev")
	if err != nil {
		t.Fatalf("resolve compose files: %v", err)
	}
	if result.SetCount != 1 {
		t.Fatalf("expected set count 1, got %d", result.SetCount)
	}
	if len(result.Files) != 1 || result.Files[0] != configFile {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRunProvisionerSkipsServicePrecheckWhenResolvedFilesExist(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := writeTestComposeFile(configFile, "services: {}"); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	client := &provisionerDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					compose.ComposeProjectLabel:     "esb-dev",
					compose.ComposeConfigFilesLabel: configFile,
				},
			},
		},
	}
	runner := &provisionerRunner{}
	p := newComposeProvisioner(runner, nil, func() (compose.DockerClient, error) {
		return client, nil
	})

	projectDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := p.RunProvisioner("esb-dev", compose.ModeDocker, true, false, projectDir, nil); err != nil {
		t.Fatalf("run provisioner: %v", err)
	}
	if runner.serviceChecks != 0 {
		t.Fatalf("expected no compose service precheck, got %d", runner.serviceChecks)
	}
	if runner.runQuietCalls != 2 {
		t.Fatalf("expected two quiet runs (build + run), got %d", runner.runQuietCalls)
	}
}

func TestRunProvisionerParityExpectsSharedRunErrorClass(t *testing.T) {
	root := t.TempDir()
	if err := writeTestComposeFile(filepath.Join(root, "docker-compose.docker.yml"), "services: {}\n"); err != nil {
		t.Fatalf("write repo root marker: %v", err)
	}
	composePath := filepath.Join(root, "docker-compose.yml")
	if err := writeTestComposeFile(composePath, "services:\n  provisioner: {}\n"); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	runner := &provisionerRunner{
		runOutputFn: func(args []string) ([]byte, error) {
			if containsArg(args, "--help") {
				return []byte("--no-warn-orphans"), nil
			}
			if containsArg(args, "config") && containsArg(args, "--services") {
				return []byte("provisioner\n"), nil
			}
			return []byte{}, nil
		},
		runQuietErr: errors.New("compose failed"),
	}
	p := newComposeProvisioner(runner, nil, nil)

	err := p.RunProvisioner("esb-dev", compose.ModeDocker, true, false, root, []string{composePath})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "run provisioner") {
		t.Fatalf("expected shared run error class, got: %v", err)
	}
}

func TestDefaultComposeFiles(t *testing.T) {
	root := t.TempDir()
	dockerProxy := filepath.Join(root, "docker-compose.proxy.docker.yml")
	if err := writeTestComposeFile(dockerProxy, "services: {}"); err != nil {
		t.Fatalf("write docker proxy file: %v", err)
	}
	containerdProxy := filepath.Join(root, "docker-compose.proxy.containerd.yml")
	if err := writeTestComposeFile(containerdProxy, "services: {}"); err != nil {
		t.Fatalf("write containerd proxy file: %v", err)
	}

	dockerFiles := defaultComposeFiles(root, compose.ModeDocker)
	if len(dockerFiles) != 2 {
		t.Fatalf("expected 2 docker files, got %d", len(dockerFiles))
	}
	if dockerFiles[0] != filepath.Join(root, "docker-compose.docker.yml") {
		t.Fatalf("unexpected docker base file: %s", dockerFiles[0])
	}
	if dockerFiles[1] != dockerProxy {
		t.Fatalf("unexpected docker proxy file: %s", dockerFiles[1])
	}

	containerdFiles := defaultComposeFiles(root, compose.ModeContainerd)
	if len(containerdFiles) != 2 {
		t.Fatalf("expected 2 containerd files, got %d", len(containerdFiles))
	}
	if containerdFiles[0] != filepath.Join(root, "docker-compose.containerd.yml") {
		t.Fatalf("unexpected containerd base file: %s", containerdFiles[0])
	}
	if containerdFiles[1] != containerdProxy {
		t.Fatalf("unexpected containerd proxy file: %s", containerdFiles[1])
	}
}

func TestComposeSupportsNoWarnOrphans(t *testing.T) {
	p := composeProvisioner{
		composeRunner: &provisionerRunner{
			runOutputFn: func(args []string) ([]byte, error) {
				if containsArg(args, "--help") {
					return []byte("  --no-warn-orphans  Do not warn"), nil
				}
				return []byte{}, nil
			},
		},
	}
	if !p.composeSupportsNoWarnOrphans("/tmp") {
		t.Fatal("expected --no-warn-orphans support")
	}
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func writeTestComposeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
