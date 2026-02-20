// Where: cli/internal/command/deploy_running_projects_test.go
// What: Unit tests for deploy running project discovery helpers.
// Why: Ensure project filtering and env inference from labels behave deterministically.
package command

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
	runtimecfg "github.com/poruru-code/esb/cli/internal/domain/runtime"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
	"github.com/poruru-code/esb/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru-code/esb/cli/internal/infra/runtime"
)

func TestInferEnvFromContainerLabels(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
		}
		got := runtimeinfra.InferEnvFromContainerLabels(containers)
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
		got := runtimeinfra.InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})

	t.Run("none", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{}},
		}
		got := runtimeinfra.InferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})
}

func TestExtractRunningDeployTargetStacks(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-gateway"},
			Labels: map[string]string{compose.ComposeServiceLabel: "gateway", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-infra-registry"},
			Labels: map[string]string{compose.ComposeServiceLabel: "registry", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/buildx_buildkit_esb-buildx0"},
			Labels: map[string]string{},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "esb3", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestExtractRunningDeployTargetStacksGatewayDown(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-dev-database"},
			Labels: map[string]string{compose.ComposeServiceLabel: "database", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-infra-registry"},
			Labels: map[string]string{compose.ComposeServiceLabel: "registry", compose.ComposeProjectLabel: "esb-infra"},
		},
		{
			Names:  []string{"/buildx_buildkit_esb-buildx0"},
			Labels: map[string]string{},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "esb3", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestExtractRunningDeployTargetStacksPrefersGatewayMetadata(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "project-from-agent"},
		},
		{
			Names:  []string{"/esb-dev-gateway"},
			Labels: map[string]string{compose.ComposeServiceLabel: "gateway", compose.ComposeProjectLabel: "project-from-gateway"},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "project-from-gateway", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestResolveDeployTargetStackSingleAutoSelect(t *testing.T) {
	prompter := &recordingPrompter{}
	stack, err := resolveDeployTargetStack(
		[]deployTargetStack{{Name: "esb-dev", Project: "esb3", Env: "dev"}},
		true,
		prompter,
	)
	if err != nil {
		t.Fatalf("resolve target stack: %v", err)
	}
	if stack.Name != "esb-dev" {
		t.Fatalf("unexpected stack selected: %q", stack.Name)
	}
	if prompter.selectCalls != 0 {
		t.Fatalf("expected no prompt for single stack, got %d calls", prompter.selectCalls)
	}
}

func TestResolveDeployTargetStackMultipleNonTTY(t *testing.T) {
	_, err := resolveDeployTargetStack(
		[]deployTargetStack{
			{Name: "esb-dev", Project: "proj-a", Env: "dev"},
			{Name: "esb-prod", Project: "proj-b", Env: "prod"},
		},
		false,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for multiple stacks without tty")
	}
	if !strings.Contains(err.Error(), errMultipleRunningProjects.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveDeployTargetStackMultipleTTYSelection(t *testing.T) {
	prompter := &recordingPrompter{selected: "esb-prod"}
	got, err := resolveDeployTargetStack(
		[]deployTargetStack{
			{Name: "esb-dev", Project: "proj-a", Env: "dev"},
			{Name: "esb-prod", Project: "proj-b", Env: "prod"},
		},
		true,
		prompter,
	)
	if err != nil {
		t.Fatalf("resolve target stack: %v", err)
	}
	if got.Name != "esb-prod" || got.Project != "proj-b" {
		t.Fatalf("unexpected selected stack: %#v", got)
	}
}

func TestResolveDeployTargetStackMultipleTTYUnknownSelection(t *testing.T) {
	prompter := &recordingPrompter{selected: "does-not-exist"}
	got, err := resolveDeployTargetStack(
		[]deployTargetStack{
			{Name: "esb-dev", Project: "proj-a", Env: "dev"},
			{Name: "esb-prod", Project: "proj-b", Env: "prod"},
		},
		true,
		prompter,
	)
	if err != nil {
		t.Fatalf("resolve target stack: %v", err)
	}
	if got != (deployTargetStack{}) {
		t.Fatalf("expected empty stack for unknown selection, got %#v", got)
	}
}

func TestInferEnvFromStackName(t *testing.T) {
	if got := inferEnvFromStackName("esb-dev"); got != "dev" {
		t.Fatalf("expected dev env, got %q", got)
	}
	if got := inferEnvFromStackName("esb-e2e-docker"); got != "e2e-docker" {
		t.Fatalf("expected e2e-docker env, got %q", got)
	}
	if got := inferEnvFromStackName("esb"); got != "" {
		t.Fatalf("expected empty env, got %q", got)
	}
}

func TestInferStackFromServiceName(t *testing.T) {
	if got := inferStackFromServiceName("esb-dev-runtime-node", "runtime-node"); got != "esb-dev" {
		t.Fatalf("expected esb-dev stack, got %q", got)
	}
	if got := inferStackFromServiceName("esb-dev-gateway", "agent"); got != "" {
		t.Fatalf("expected empty stack, got %q", got)
	}
	if got := inferStackFromServiceName("", "gateway"); got != "" {
		t.Fatalf("expected empty stack for blank name, got %q", got)
	}
}

func TestNewDockerClientRejectsNilFactory(t *testing.T) {
	_, err := newDockerClient(nil)
	if err == nil {
		t.Fatal("expected nil factory error")
	}
	if !strings.Contains(err.Error(), "docker client factory is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDockerClientRejectsFactoryErrorAndNilClient(t *testing.T) {
	_, err := newDockerClient(func() (compose.DockerClient, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected factory error")
	}
	if !strings.Contains(err.Error(), "create docker client") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = newDockerClient(func() (compose.DockerClient, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected nil client error")
	}
	if !strings.Contains(err.Error(), "returned nil client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContainerInfos(t *testing.T) {
	if got := containerInfos(nil); got != nil {
		t.Fatalf("expected nil result, got %#v", got)
	}

	containers := []container.Summary{
		{
			State: " running ",
			Labels: map[string]string{
				compose.ComposeServiceLabel: " gateway ",
			},
		},
		{
			State:  "exited",
			Labels: nil,
		},
	}
	got := containerInfos(containers)
	want := []runtimecfg.ContainerInfo{
		{Service: "gateway", State: "running"},
		{Service: "", State: "exited"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected container infos: got=%#v want=%#v", got, want)
	}
}

type stackDockerClient struct {
	containers     []container.Summary
	listErr        error
	listErrOnCall  int
	containerCalls int
}

func (c *stackDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	c.containerCalls++
	if c.listErrOnCall > 0 && c.containerCalls == c.listErrOnCall {
		return nil, errors.New("boom on call")
	}
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.containers, nil
}

func (c *stackDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c *stackDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c *stackDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c *stackDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c *stackDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c *stackDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c *stackDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c *stackDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

func TestDiscoverRunningDeployTargetStacks(t *testing.T) {
	client := &stackDockerClient{
		containers: []container.Summary{
			{
				Names:  []string{"/esb-dev-gateway"},
				Labels: map[string]string{compose.ComposeServiceLabel: "gateway", compose.ComposeProjectLabel: "esb-dev"},
				State:  "running",
			},
		},
	}
	got, err := discoverRunningDeployTargetStacks(func() (compose.DockerClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("discover running stacks: %v", err)
	}
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "esb-dev", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: got=%#v want=%#v", got, want)
	}
}

func TestDiscoverRunningDeployTargetStacksPropagatesListError(t *testing.T) {
	_, err := discoverRunningDeployTargetStacks(func() (compose.DockerClient, error) {
		return &stackDockerClient{listErr: errors.New("boom")}, nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list containers: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInferDeployModeFromProjectFromRunningServices(t *testing.T) {
	client := &stackDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					compose.ComposeProjectLabel: "esb-dev",
					compose.ComposeServiceLabel: "runtime-node",
				},
			},
		},
	}
	mode, source, err := inferDeployModeFromProject("esb-dev", func() (compose.DockerClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("infer deploy mode: %v", err)
	}
	if mode != runtimecfg.ModeContainerd || source != "running_services" {
		t.Fatalf("unexpected mode/source: mode=%q source=%q", mode, source)
	}
}

func TestInferDeployModeFromProjectFromComposeFiles(t *testing.T) {
	root := t.TempDir()
	composeFile := filepath.Join(root, "docker-compose.containerd.yml")
	if err := os.WriteFile(composeFile, []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	client := &stackDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					compose.ComposeProjectLabel:     "esb-dev",
					compose.ComposeConfigFilesLabel: filepath.Base(composeFile),
					compose.ComposeWorkingDirLabel:  root,
				},
			},
		},
	}

	mode, source, err := inferDeployModeFromProject("esb-dev", func() (compose.DockerClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("infer deploy mode: %v", err)
	}
	if mode != runtimecfg.ModeContainerd || source != "config_files" {
		t.Fatalf("unexpected mode/source: mode=%q source=%q", mode, source)
	}
}

func TestInferDeployModeFromProjectWrapsResolveComposeFilesError(t *testing.T) {
	client := &stackDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					compose.ComposeProjectLabel: "esb-dev",
					compose.ComposeServiceLabel: "database",
				},
			},
		},
		listErrOnCall: 2,
	}
	_, _, err := inferDeployModeFromProject("esb-dev", func() (compose.DockerClient, error) {
		return client, nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resolve compose files: list containers: boom on call") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordingPrompter struct {
	inputCalls  int
	inputValue  string
	selectCalls int
	selected    string
}

func (p *recordingPrompter) Input(_ string, _ []string) (string, error) {
	p.inputCalls++
	return p.inputValue, nil
}

func (p *recordingPrompter) Select(_ string, _ []string) (string, error) {
	p.selectCalls++
	if p.selected != "" {
		return p.selected, nil
	}
	return "", nil
}

func (p *recordingPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return "", nil
}
