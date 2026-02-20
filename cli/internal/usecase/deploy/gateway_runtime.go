// Where: cli/internal/usecase/deploy/gateway_runtime.go
// What: Gateway runtime detection and alignment for deploy workflow.
// Why: Keep runtime/container probing concerns separate from deploy orchestration.
package deploy

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/poruru-code/esb/cli/internal/constants"
	"github.com/poruru-code/esb/cli/internal/domain/value"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
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
	info, err := w.resolveGatewayRuntime(req.Context.ComposeProject)
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

func (w Workflow) resolveGatewayRuntime(composeProject string) (gatewayRuntimeInfo, error) {
	if w.DockerClient == nil {
		return gatewayRuntimeInfo{}, nil
	}
	client, err := w.newDockerClient()
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("create docker client: %w", err)
	}

	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
	if strings.TrimSpace(composeProject) != "" {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	}
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("list containers: %w", err)
	}
	if len(containers) == 0 {
		return gatewayRuntimeInfo{}, nil
	}
	sort.SliceStable(containers, func(i, j int) bool {
		iRunning := strings.EqualFold(containers[i].State, "running")
		jRunning := strings.EqualFold(containers[j].State, "running")
		if iRunning != jRunning {
			return iRunning
		}
		return gatewayContainerName(containers[i]) < gatewayContainerName(containers[j])
	})
	selected := containers[0]
	inspect, err := client.ContainerInspect(ctx, selected.ID)
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("inspect container: %w", err)
	}
	envValues := []string{}
	if inspect.Config != nil {
		envValues = inspect.Config.Env
	}
	envMap := value.EnvSliceToMap(envValues)
	info := gatewayRuntimeInfo{
		ComposeProject:    strings.TrimSpace(selected.Labels[compose.ComposeProjectLabel]),
		ProjectName:       strings.TrimSpace(envMap[constants.EnvProjectName]),
		ContainersNetwork: strings.TrimSpace(envMap["CONTAINERS_NETWORK"]),
	}
	if info.ContainersNetwork == "" && inspect.NetworkSettings != nil {
		info.ContainersNetwork = pickGatewayNetwork(inspect.NetworkSettings.Networks)
	}
	return info, nil
}

func pickGatewayNetwork(networks map[string]*network.EndpointSettings) string {
	if len(networks) == 0 {
		return ""
	}
	names := make([]string, 0, len(networks))
	for name := range networks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.Contains(name, "external") {
			return name
		}
	}
	if len(names) > 0 {
		return names[0]
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
	if w.DockerClient == nil {
		return
	}
	client, err := w.newDockerClient()
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
	missingSet := make(map[string]struct{}, len(required))
	for _, ctr := range containers {
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		if _, ok := required[service]; !ok {
			continue
		}
		if !containerOnNetwork(&ctr, gatewayNetwork) {
			missingSet[service] = struct{}{}
		}
	}
	missing := make([]string, 0, len(missingSet))
	for service := range missingSet {
		missing = append(missing, service)
	}
	sort.Strings(missing)
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

func (w Workflow) newDockerClient() (compose.DockerClient, error) {
	if w.DockerClient == nil {
		return nil, errDockerClientNotConfigured
	}
	client, err := w.DockerClient()
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errDockerClientNotConfigured
	}
	return client, nil
}

func gatewayContainerName(summary container.Summary) string {
	return compose.PrimaryContainerName(summary.Names)
}
