// Where: cli/internal/usecase/deploy/gateway_runtime_test.go
// What: Deterministic and branch tests for gateway runtime helpers.
// Why: Prevent nondeterminism and ensure warning paths stay observable.
package deploy

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/poruru-code/esb/cli/internal/constants"
	"github.com/poruru-code/esb/cli/internal/domain/state"
	"github.com/poruru-code/esb/cli/internal/domain/value"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

func TestPickGatewayNetworkPrefersExternalAndSorts(t *testing.T) {
	networks := map[string]*network.EndpointSettings{
		"zeta-external":  {},
		"alpha-external": {},
		"default":        {},
	}
	got := pickGatewayNetwork(networks)
	if got != "alpha-external" {
		t.Fatalf("expected first sorted external network, got %q", got)
	}
}

func TestPickGatewayNetworkFallsBackToSortedName(t *testing.T) {
	networks := map[string]*network.EndpointSettings{
		"zeta":  {},
		"alpha": {},
	}
	got := pickGatewayNetwork(networks)
	if got != "alpha" {
		t.Fatalf("expected first sorted network, got %q", got)
	}
}

func TestResolveGatewayRuntimeSkipsWhenDockerClientNotConfigured(t *testing.T) {
	workflow := Workflow{}
	info, err := workflow.resolveGatewayRuntime("esb-dev")
	if err != nil {
		t.Fatalf("resolve gateway runtime: %v", err)
	}
	if info != (gatewayRuntimeInfo{}) {
		t.Fatalf("expected empty gateway runtime info, got %+v", info)
	}
}

func TestResolveGatewayRuntimeWithEmptyProjectListsContainersOnce(t *testing.T) {
	client := &countingGatewayDockerClient{}
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return client, nil
		},
	}

	info, err := workflow.resolveGatewayRuntime("")
	if err != nil {
		t.Fatalf("resolve gateway runtime: %v", err)
	}
	if info != (gatewayRuntimeInfo{}) {
		t.Fatalf("expected empty gateway runtime info, got %+v", info)
	}
	if client.listCalls != 1 {
		t.Fatalf("expected one container list call, got %d", client.listCalls)
	}
}

func TestResolveGatewayRuntimeHandlesNilInspectFields(t *testing.T) {
	client := &inspectGatewayDockerClient{
		containers: []container.Summary{
			{
				ID:    "ctr-1",
				State: "running",
				Labels: map[string]string{
					compose.ComposeServiceLabel: "gateway",
					compose.ComposeProjectLabel: "esb-dev",
				},
			},
		},
		inspect: container.InspectResponse{},
	}
	workflow := Workflow{
		DockerClient: func() (compose.DockerClient, error) {
			return client, nil
		},
	}

	info, err := workflow.resolveGatewayRuntime("esb-dev")
	if err != nil {
		t.Fatalf("resolve gateway runtime: %v", err)
	}
	if info.ComposeProject != "esb-dev" {
		t.Fatalf("expected compose project esb-dev, got %q", info.ComposeProject)
	}
	if info.ProjectName != "" {
		t.Fatalf("expected empty project name, got %q", info.ProjectName)
	}
	if info.ContainersNetwork != "" {
		t.Fatalf("expected empty network, got %q", info.ContainersNetwork)
	}
}

func TestAlignGatewayRuntimeAppliesDetectedProjectAndEnv(t *testing.T) {
	t.Setenv(constants.EnvProjectName, "")
	t.Setenv(constants.EnvNetworkExternal, "")
	client := &inspectGatewayDockerClient{
		containers: []container.Summary{
			{
				ID:    "ctr-1",
				State: "running",
				Labels: map[string]string{
					compose.ComposeServiceLabel: "gateway",
					compose.ComposeProjectLabel: "esb-live",
				},
			},
		},
		inspect: container.InspectResponse{
			Config: &container.Config{
				Env: []string{
					constants.EnvProjectName + "=esb-live",
					"CONTAINERS_NETWORK=external-net",
				},
			},
		},
	}
	ui := &testUI{}
	workflow := Workflow{
		UserInterface: ui,
		DockerClient: func() (compose.DockerClient, error) {
			return client, nil
		},
	}
	req := Request{
		Context: state.Context{
			ComposeProject: "esb-dev",
		},
	}

	got := workflow.alignGatewayRuntime(req)
	if got.Context.ComposeProject != "esb-live" {
		t.Fatalf("expected compose project esb-live, got %q", got.Context.ComposeProject)
	}
	if envProject := strings.TrimSpace(os.Getenv(constants.EnvProjectName)); envProject != "esb-live" {
		t.Fatalf("expected %s=esb-live, got %q", constants.EnvProjectName, envProject)
	}
	if envNetwork := strings.TrimSpace(os.Getenv(constants.EnvNetworkExternal)); envNetwork != "external-net" {
		t.Fatalf("expected %s=external-net, got %q", constants.EnvNetworkExternal, envNetwork)
	}
	if len(ui.warn) == 0 || !strings.Contains(ui.warn[0], "using running gateway project") {
		t.Fatalf("expected project replacement warning, got %#v", ui.warn)
	}
}

func TestAlignGatewayRuntimeWarnsOnResolveFailure(t *testing.T) {
	ui := &testUI{}
	workflow := Workflow{
		UserInterface: ui,
		DockerClient: func() (compose.DockerClient, error) {
			return nil, nil
		},
	}
	req := Request{
		Context: state.Context{
			ComposeProject: "esb-dev",
		},
	}

	got := workflow.alignGatewayRuntime(req)
	if got.Context.ComposeProject != req.Context.ComposeProject {
		t.Fatalf("compose project changed unexpectedly: %q", got.Context.ComposeProject)
	}
	if len(ui.warn) != 1 {
		t.Fatalf("expected one warning, got %d", len(ui.warn))
	}
	if !strings.Contains(ui.warn[0], "failed to resolve gateway runtime") {
		t.Fatalf("unexpected warning: %q", ui.warn[0])
	}
}

func TestEnvSliceToMap(t *testing.T) {
	got := value.EnvSliceToMap([]string{
		"A=1",
		" B =2",
		"NO_EQUALS",
		"",
		"=skip",
		"A=override",
	})
	if got["A"] != "override" {
		t.Fatalf("expected A override, got %q", got["A"])
	}
	if got["B"] != "2" {
		t.Fatalf("expected B=2, got %q", got["B"])
	}
	if got["NO_EQUALS"] != "" {
		t.Fatalf("expected NO_EQUALS empty value, got %q", got["NO_EQUALS"])
	}
	if _, ok := got[""]; ok {
		t.Fatalf("empty key must be ignored")
	}
}

func TestContainerOnNetwork(t *testing.T) {
	if containerOnNetwork(nil, "external") {
		t.Fatalf("nil container must not be on network")
	}

	ctr := &container.Summary{
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*network.EndpointSettings{
				"external": {},
			},
		},
	}
	if !containerOnNetwork(ctr, "external") {
		t.Fatalf("expected container on external network")
	}
	if containerOnNetwork(ctr, "other") {
		t.Fatalf("unexpected network match")
	}
}

func TestGatewayContainerName(t *testing.T) {
	got := gatewayContainerName(container.Summary{
		Names: []string{"/gateway", " /ignored"},
	})
	if got != "gateway" {
		t.Fatalf("expected gateway, got %q", got)
	}

	empty := gatewayContainerName(container.Summary{
		Names: []string{"", "   ", "/"},
	})
	if empty != "" {
		t.Fatalf("expected empty name, got %q", empty)
	}
}

func TestWarnInfraNetworkMismatchReportsMissingServices(t *testing.T) {
	ui := &testUI{}
	workflow := Workflow{
		UserInterface: ui,
		DockerClient: func() (compose.DockerClient, error) {
			return mismatchDockerClient{
				containers: []container.Summary{
					{
						Labels: map[string]string{
							compose.ComposeServiceLabel: "database",
						},
						NetworkSettings: &container.NetworkSettingsSummary{
							Networks: map[string]*network.EndpointSettings{
								"not-external": {},
							},
						},
					},
					{
						Labels: map[string]string{
							compose.ComposeServiceLabel: "s3-storage",
						},
						NetworkSettings: &container.NetworkSettingsSummary{
							Networks: map[string]*network.EndpointSettings{
								"external": {},
							},
						},
					},
					{
						Labels: map[string]string{
							compose.ComposeServiceLabel: "victorialogs",
						},
						NetworkSettings: &container.NetworkSettingsSummary{
							Networks: map[string]*network.EndpointSettings{
								"other": {},
							},
						},
					},
				},
			}, nil
		},
	}

	workflow.warnInfraNetworkMismatch("esb-dev", "external")

	if len(ui.warn) != 1 {
		t.Fatalf("expected one warning, got %d", len(ui.warn))
	}
	msg := ui.warn[0]
	if !strings.Contains(msg, "database, victorialogs") {
		t.Fatalf("expected sorted missing services in warning, got %q", msg)
	}
}

type mismatchDockerClient struct {
	containers []container.Summary
}

func (c mismatchDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return c.containers, nil
}

func (c mismatchDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c mismatchDockerClient) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (c mismatchDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (c mismatchDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (c mismatchDockerClient) ContainersPrune(_ context.Context, _ filters.Args) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c mismatchDockerClient) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c mismatchDockerClient) NetworksPrune(_ context.Context, _ filters.Args) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c mismatchDockerClient) VolumesPrune(_ context.Context, _ filters.Args) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

type countingGatewayDockerClient struct {
	listCalls int
}

func (c *countingGatewayDockerClient) ContainerList(
	_ context.Context,
	_ container.ListOptions,
) ([]container.Summary, error) {
	c.listCalls++
	return nil, nil
}

func (c *countingGatewayDockerClient) ContainerInspect(
	_ context.Context,
	_ string,
) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (c *countingGatewayDockerClient) ImageList(
	_ context.Context,
	_ image.ListOptions,
) ([]image.Summary, error) {
	return nil, nil
}

func (c *countingGatewayDockerClient) ContainerStop(
	_ context.Context,
	_ string,
	_ container.StopOptions,
) error {
	return nil
}

func (c *countingGatewayDockerClient) ContainerRemove(
	_ context.Context,
	_ string,
	_ container.RemoveOptions,
) error {
	return nil
}

func (c *countingGatewayDockerClient) ContainersPrune(
	_ context.Context,
	_ filters.Args,
) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c *countingGatewayDockerClient) ImagesPrune(
	_ context.Context,
	_ filters.Args,
) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c *countingGatewayDockerClient) NetworksPrune(
	_ context.Context,
	_ filters.Args,
) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c *countingGatewayDockerClient) VolumesPrune(
	_ context.Context,
	_ filters.Args,
) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}

type inspectGatewayDockerClient struct {
	containers []container.Summary
	inspect    container.InspectResponse
}

func (c *inspectGatewayDockerClient) ContainerList(
	_ context.Context,
	_ container.ListOptions,
) ([]container.Summary, error) {
	return c.containers, nil
}

func (c *inspectGatewayDockerClient) ContainerInspect(
	_ context.Context,
	_ string,
) (container.InspectResponse, error) {
	return c.inspect, nil
}

func (c *inspectGatewayDockerClient) ImageList(
	_ context.Context,
	_ image.ListOptions,
) ([]image.Summary, error) {
	return nil, nil
}

func (c *inspectGatewayDockerClient) ContainerStop(
	_ context.Context,
	_ string,
	_ container.StopOptions,
) error {
	return nil
}

func (c *inspectGatewayDockerClient) ContainerRemove(
	_ context.Context,
	_ string,
	_ container.RemoveOptions,
) error {
	return nil
}

func (c *inspectGatewayDockerClient) ContainersPrune(
	_ context.Context,
	_ filters.Args,
) (container.PruneReport, error) {
	return container.PruneReport{}, nil
}

func (c *inspectGatewayDockerClient) ImagesPrune(
	_ context.Context,
	_ filters.Args,
) (image.PruneReport, error) {
	return image.PruneReport{}, nil
}

func (c *inspectGatewayDockerClient) NetworksPrune(
	_ context.Context,
	_ filters.Args,
) (network.PruneReport, error) {
	return network.PruneReport{}, nil
}

func (c *inspectGatewayDockerClient) VolumesPrune(
	_ context.Context,
	_ filters.Args,
) (volume.PruneReport, error) {
	return volume.PruneReport{}, nil
}
