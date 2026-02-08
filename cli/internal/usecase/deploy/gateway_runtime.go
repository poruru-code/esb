// Where: cli/internal/usecase/deploy/gateway_runtime.go
// What: Gateway runtime detection and alignment for deploy workflow.
// Why: Keep runtime/container probing concerns separate from deploy orchestration.
package deploy

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

type gatewayRuntimeInfo struct {
	ComposeProject    string
	ProjectName       string
	ContainersNetwork string
}

func (w Workflow) alignGatewayRuntime(req Request) Request {
	if skipGatewayAlign() {
		return req
	}
	info, err := resolveGatewayRuntime(req.Context.ComposeProject)
	if err != nil {
		if w.UserInterface != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to resolve gateway runtime: %v", err))
		}
		return req
	}
	if info.ComposeProject != "" && info.ComposeProject != req.Context.ComposeProject {
		if w.UserInterface != nil {
			w.UserInterface.Warn(
				fmt.Sprintf("Warning: using running gateway project %q (was %q)", info.ComposeProject, req.Context.ComposeProject),
			)
		}
		req.Context.ComposeProject = info.ComposeProject
	}
	if info.ProjectName != "" && strings.TrimSpace(os.Getenv(constants.EnvProjectName)) != info.ProjectName {
		_ = os.Setenv(constants.EnvProjectName, info.ProjectName)
	}
	if info.ContainersNetwork != "" && strings.TrimSpace(os.Getenv(constants.EnvNetworkExternal)) != info.ContainersNetwork {
		_ = os.Setenv(constants.EnvNetworkExternal, info.ContainersNetwork)
	}
	w.warnInfraNetworkMismatch(req.Context.ComposeProject, info.ContainersNetwork)
	return req
}

func skipGatewayAlign() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ESB_SKIP_GATEWAY_ALIGN")))
	return value == "1" || value == "true" || value == "yes"
}

func resolveGatewayRuntime(composeProject string) (gatewayRuntimeInfo, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("create docker client: %w", err)
	}
	rawClient, ok := client.(*dockerclient.Client)
	if !ok {
		return gatewayRuntimeInfo{}, errUnsupportedDockerClient
	}

	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
	if strings.TrimSpace(composeProject) != "" {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	}
	containers, err := rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("list containers: %w", err)
	}
	if len(containers) == 0 && strings.TrimSpace(composeProject) == "" {
		filterArgs = filters.NewArgs()
		filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
		containers, err = rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
		if err != nil {
			return gatewayRuntimeInfo{}, fmt.Errorf("list containers: %w", err)
		}
	}
	if len(containers) == 0 {
		return gatewayRuntimeInfo{}, nil
	}

	selected := containers[0]
	for _, ctr := range containers {
		if strings.EqualFold(ctr.State, "running") {
			selected = ctr
			break
		}
	}
	inspect, err := rawClient.ContainerInspect(ctx, selected.ID)
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("inspect container: %w", err)
	}
	envMap := envSliceToMap(inspect.Config.Env)
	info := gatewayRuntimeInfo{
		ComposeProject:    strings.TrimSpace(selected.Labels[compose.ComposeProjectLabel]),
		ProjectName:       strings.TrimSpace(envMap[constants.EnvProjectName]),
		ContainersNetwork: strings.TrimSpace(envMap["CONTAINERS_NETWORK"]),
	}
	if info.ContainersNetwork == "" {
		info.ContainersNetwork = pickGatewayNetwork(inspect.NetworkSettings.Networks)
	}
	return info, nil
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		out[key] = value
	}
	return out
}

func pickGatewayNetwork(networks map[string]*network.EndpointSettings) string {
	for name := range networks {
		if strings.Contains(name, "external") {
			return name
		}
	}
	for name := range networks {
		return name
	}
	return ""
}

func (w Workflow) warnInfraNetworkMismatch(composeProject, gatewayNetwork string) {
	if w.UserInterface == nil {
		return
	}
	if strings.TrimSpace(gatewayNetwork) == "" {
		return
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	if strings.TrimSpace(composeProject) != "" {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	}
	containers, err := client.ContainerList(ctx, container.ListOptions{All: false, Filters: filterArgs})
	if err != nil {
		return
	}
	required := map[string]struct{}{
		"database":     {},
		"s3-storage":   {},
		"victorialogs": {},
	}
	missing := make([]string, 0, len(required))
	for _, ctr := range containers {
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		if _, ok := required[service]; !ok {
			continue
		}
		if !containerOnNetwork(&ctr, gatewayNetwork) {
			missing = append(missing, service)
		}
	}
	if len(missing) == 0 {
		return
	}
	w.UserInterface.Warn(
		fmt.Sprintf(
			"Warning: gateway network %q is missing services: %s. Recreate the stack or attach the services to that network.",
			gatewayNetwork,
			strings.Join(missing, ", "),
		),
	)
}

func containerOnNetwork(ctr *container.Summary, network string) bool {
	if ctr == nil || ctr.NetworkSettings == nil {
		return false
	}
	for name := range ctr.NetworkSettings.Networks {
		if name == network {
			return true
		}
	}
	return false
}
