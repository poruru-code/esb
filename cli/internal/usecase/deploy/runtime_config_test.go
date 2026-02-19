// Where: cli/internal/usecase/deploy/runtime_config_test.go
// What: Unit tests for runtime config sync error handling.
// Why: Prevent silent success when container copy fails.
package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

func TestSyncRuntimeConfigToTarget_ContainerCopyFailureIsReturned(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	runner := &runtimeConfigRunner{
		runFunc: func(args ...string) error {
			if len(args) > 0 && args[0] == "cp" {
				return errors.New("container copy failed")
			}
			return nil
		},
	}
	workflow := Workflow{ComposeRunner: runner}

	err := workflow.syncRuntimeConfigToTarget(stagingDir, runtimeConfigTarget{ContainerID: "ctr-1"})
	if err == nil {
		t.Fatal("expected sync error, got nil")
	}
	if !strings.Contains(err.Error(), "copy config to container") {
		t.Fatalf("expected container copy context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "container copy failed") {
		t.Fatalf("expected underlying container error, got: %v", err)
	}
}

func TestSyncRuntimeConfigFromDirSkipsWhenComposeProjectEmpty(t *testing.T) {
	called := false
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			called = true
			return runtimeConfigDockerClient{}, nil
		},
	}
	if err := workflow.syncRuntimeConfigFromDir("", t.TempDir()); err != nil {
		t.Fatalf("syncRuntimeConfigFromDir() error = %v, want nil", err)
	}
	if called {
		t.Fatal("docker client must not be called when compose project is empty")
	}
}

func TestSyncRuntimeConfigFromDirRequiresStagingDir(t *testing.T) {
	workflow := Workflow{}
	err := workflow.syncRuntimeConfigFromDir("esb-dev", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "staging dir is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncRuntimeConfigFromDirSkipsWhenStagingDirMissing(t *testing.T) {
	workflow := Workflow{}
	missing := filepath.Join(t.TempDir(), "missing")
	if err := workflow.syncRuntimeConfigFromDir("esb-dev", missing); err != nil {
		t.Fatalf("syncRuntimeConfigFromDir() error = %v, want nil", err)
	}
}

func TestSyncRuntimeConfigFromDirPropagatesResolveTargetError(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return nil, errors.New("docker unavailable")
		},
	}

	err := workflow.syncRuntimeConfigFromDir("esb-dev", t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create docker client") {
		t.Fatalf("expected create docker client context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "docker unavailable") {
		t.Fatalf("expected underlying error in message, got: %v", err)
	}
}

func TestSyncRuntimeConfigFromDirCopiesToResolvedBindPath(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("functions:\n  a: {}\n"), 0o644); err != nil {
		t.Fatalf("write functions.yml: %v", err)
	}
	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "functions.yml")
	if err := os.WriteFile(destFile, []byte("functions:\n  old: {}\n"), 0o644); err != nil {
		t.Fatalf("seed destination file: %v", err)
	}

	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeConfigDockerClient{
				containers: []container.Summary{
					{
						ID:    "gateway-1",
						State: "running",
						Labels: map[string]string{
							compose.ComposeServiceLabel: "gateway",
						},
						Mounts: []container.MountPoint{
							{
								Destination: runtimeConfigMountPath,
								Type:        "bind",
								Source:      destDir,
							},
						},
					},
				},
			}, nil
		},
	}

	if err := workflow.syncRuntimeConfigFromDir("esb-dev", stagingDir); err != nil {
		t.Fatalf("syncRuntimeConfigFromDir() error = %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(got) != "functions:\n  a: {}\n" {
		t.Fatalf("unexpected functions.yml content: %q", string(got))
	}
}

func TestSyncRuntimeConfigToTarget_JoinsContainerAndVolumeErrors(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	runner := &runtimeConfigRunner{
		runFunc: func(args ...string) error {
			if len(args) == 0 {
				return nil
			}
			switch args[0] {
			case "cp":
				return errors.New("container copy failed")
			case "run":
				return errors.New("volume copy failed")
			default:
				return nil
			}
		},
	}
	workflow := Workflow{ComposeRunner: runner}

	err := workflow.syncRuntimeConfigToTarget(
		stagingDir,
		runtimeConfigTarget{
			ContainerID: "ctr-1",
			VolumeName:  "vol-1",
		},
	)
	if err == nil {
		t.Fatal("expected sync error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "container copy failed") {
		t.Fatalf("expected container error in joined message, got: %v", err)
	}
	if !strings.Contains(msg, "volume copy failed") {
		t.Fatalf("expected volume error in joined message, got: %v", err)
	}
}

func TestResolveRuntimeConfigTargetSkipsWhenDockerClientNotConfigured(t *testing.T) {
	workflow := Workflow{}
	target, err := workflow.resolveRuntimeConfigTarget("esb-dev")
	if err != nil {
		t.Fatalf("resolve runtime config target: %v", err)
	}
	if target != (runtimeConfigTarget{}) {
		t.Fatalf("expected empty target, got %+v", target)
	}
}

func TestOrderedRuntimeConfigContainersPrefersGatewayRunningThenName(t *testing.T) {
	ordered := orderedRuntimeConfigContainers([]container.Summary{
		{
			ID:    "agent-running",
			State: "running",
			Names: []string{"/z-agent"},
			Labels: map[string]string{
				compose.ComposeServiceLabel: "agent",
			},
		},
		{
			ID:    "gateway-exited",
			State: "exited",
			Names: []string{"/b-gateway"},
			Labels: map[string]string{
				compose.ComposeServiceLabel: "gateway",
			},
		},
		{
			ID:    "gateway-running-zeta",
			State: "running",
			Names: []string{"/z-gateway"},
			Labels: map[string]string{
				compose.ComposeServiceLabel: "gateway",
			},
		},
		{
			ID:    "gateway-running-alpha",
			State: "running",
			Names: []string{"/a-gateway"},
			Labels: map[string]string{
				compose.ComposeServiceLabel: "gateway",
			},
		},
	})
	if len(ordered) == 0 {
		t.Fatal("expected ordered containers, got empty result")
	}
	if ordered[0].ID != "gateway-running-alpha" {
		t.Fatalf("expected gateway-running-alpha, got %q", ordered[0].ID)
	}
}

func TestResolveRuntimeConfigTargetUsesDeterministicContainerSelection(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeConfigDockerClient{
				containers: []container.Summary{
					{
						ID:    "gateway-zeta",
						State: "running",
						Names: []string{"/z-gateway"},
						Labels: map[string]string{
							compose.ComposeServiceLabel: "gateway",
						},
						Mounts: []container.MountPoint{
							{
								Destination: runtimeConfigMountPath,
								Type:        "bind",
								Source:      "/tmp/zeta",
							},
						},
					},
					{
						ID:    "gateway-alpha",
						State: "running",
						Names: []string{"/a-gateway"},
						Labels: map[string]string{
							compose.ComposeServiceLabel: "gateway",
						},
						Mounts: []container.MountPoint{
							{
								Destination: runtimeConfigMountPath,
								Type:        "bind",
								Source:      "/tmp/alpha",
							},
						},
					},
				},
			}, nil
		},
	}

	target, err := workflow.resolveRuntimeConfigTarget("esb-dev")
	if err != nil {
		t.Fatalf("resolve runtime config target: %v", err)
	}
	if target.ContainerID != "gateway-alpha" {
		t.Fatalf("expected gateway-alpha container, got %q", target.ContainerID)
	}
	if target.BindPath != "/tmp/alpha" {
		t.Fatalf("expected bind path /tmp/alpha, got %q", target.BindPath)
	}
}

func TestResolveRuntimeConfigTargetSkipsContainersWithoutRuntimeConfigMount(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeConfigDockerClient{
				containers: []container.Summary{
					{
						ID:    "gateway-no-mount",
						State: "running",
						Labels: map[string]string{
							compose.ComposeServiceLabel: "gateway",
						},
					},
					{
						ID:    "agent-with-volume",
						State: "running",
						Labels: map[string]string{
							compose.ComposeServiceLabel: "agent",
						},
						Mounts: []container.MountPoint{
							{
								Destination: runtimeConfigMountPath,
								Type:        "volume",
								Name:        "esb-runtime-config",
							},
						},
					},
				},
			}, nil
		},
	}

	target, err := workflow.resolveRuntimeConfigTarget("esb-dev")
	if err != nil {
		t.Fatalf("resolve runtime config target: %v", err)
	}
	if target.ContainerID != "agent-with-volume" {
		t.Fatalf("expected agent-with-volume container, got %q", target.ContainerID)
	}
	if target.VolumeName != "esb-runtime-config" {
		t.Fatalf("expected volume esb-runtime-config, got %q", target.VolumeName)
	}
}

func TestResolveRuntimeConfigTargetReturnsEmptyWhenNoRuntimeConfigMountExists(t *testing.T) {
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return runtimeConfigDockerClient{
				containers: []container.Summary{
					{
						ID:    "gateway-no-mount",
						State: "running",
						Labels: map[string]string{
							compose.ComposeServiceLabel: "gateway",
						},
					},
				},
			}, nil
		},
	}

	target, err := workflow.resolveRuntimeConfigTarget("esb-dev")
	if err != nil {
		t.Fatalf("resolve runtime config target: %v", err)
	}
	if target != (runtimeConfigTarget{}) {
		t.Fatalf("expected empty target, got %+v", target)
	}
}

func TestSyncRuntimeConfigToTargetBindPathCopiesConfigFiles(t *testing.T) {
	srcDir := t.TempDir()
	destDir := filepath.Join(t.TempDir(), "runtime-config")

	expected := map[string]string{
		"functions.yml": "functions",
		"routing.yml":   "routes",
		"resources.yml": "resources",
	}
	for name, content := range expected {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	workflow := Workflow{}
	if err := workflow.syncRuntimeConfigToTarget(srcDir, runtimeConfigTarget{BindPath: destDir}); err != nil {
		t.Fatalf("sync runtime config: %v", err)
	}

	for name, want := range expected {
		got, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil {
			t.Fatalf("read copied %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("unexpected content for %s: %q", name, string(got))
		}
	}
}

func TestSamePath(t *testing.T) {
	base := t.TempDir()
	left := filepath.Join(base, ".", "runtime-config")
	right := filepath.Join(base, "runtime-config")
	if err := os.MkdirAll(right, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !samePath(left, right) {
		t.Fatalf("expected samePath to match %q and %q", left, right)
	}
}

func TestCopyFileCopiesBytes(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "src.yml")
	dest := filepath.Join(srcDir, "dest.yml")
	want := []byte("hello: world\n")

	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, dest); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dest content: %q", string(got))
	}
}

func TestCopyFileCreatesParentDirAndOverwrites(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "src.yml")
	dest := filepath.Join(srcDir, "nested", "dest.yml")

	if err := os.WriteFile(src, []byte("new: value\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir dest dir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("old: value\n"), 0o644); err != nil {
		t.Fatalf("write old dest: %v", err)
	}

	if err := copyFile(src, dest); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "new: value\n" {
		t.Fatalf("unexpected dest content: %q", string(got))
	}
}

type runtimeConfigRunner struct {
	runFunc func(args ...string) error
}

func (r *runtimeConfigRunner) Run(_ context.Context, _ string, _ string, args ...string) error {
	if r.runFunc == nil {
		return nil
	}
	return r.runFunc(args...)
}

func (r *runtimeConfigRunner) RunOutput(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (r *runtimeConfigRunner) RunQuiet(_ context.Context, _ string, _ string, args ...string) error {
	if r.runFunc == nil {
		return nil
	}
	return r.runFunc(args...)
}

type runtimeConfigDockerClient struct {
	containers []container.Summary
}

func (c runtimeConfigDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return c.containers, nil
}

func (c runtimeConfigDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c runtimeConfigDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c runtimeConfigDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c runtimeConfigDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c runtimeConfigDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c runtimeConfigDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c runtimeConfigDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c runtimeConfigDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}
