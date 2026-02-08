// Where: cli/internal/command/deploy_stack.go
// What: Deploy target stack discovery and runtime inference helpers.
// Why: Keep stack/runtime resolution concerns separate from deploy input orchestration.
package command

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	runtimecfg "github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/meta"
)

var deployStackDiscoveryServicePriority = map[string]int{
	"gateway":      0,
	"runtime-node": 1,
	"agent":        2,
	"database":     3,
	"s3-storage":   4,
	"victorialogs": 5,
	"provisioner":  6,
	"coredns":      7,
}

func resolveDeployTargetStack(
	stacks []deployTargetStack,
	isTTY bool,
	prompter interaction.Prompter,
) (deployTargetStack, error) {
	if len(stacks) == 0 {
		return deployTargetStack{}, nil
	}
	if len(stacks) == 1 {
		return stacks[0], nil
	}
	if !isTTY || prompter == nil {
		options := make([]string, 0, len(stacks))
		for _, stack := range stacks {
			options = append(options, stack.Name)
		}
		return deployTargetStack{}, fmt.Errorf("%w: %s", errMultipleRunningProjects, strings.Join(options, ", "))
	}
	options := make([]string, 0, len(stacks))
	lookup := make(map[string]deployTargetStack, len(stacks))
	for _, stack := range stacks {
		options = append(options, stack.Name)
		lookup[stack.Name] = stack
	}
	selected, err := prompter.Select("Target stack (running)", options)
	if err != nil {
		return deployTargetStack{}, fmt.Errorf("prompt target stack: %w", err)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return deployTargetStack{}, nil
	}
	stack, ok := lookup[selected]
	if !ok {
		return deployTargetStack{}, nil
	}
	return stack, nil
}

func defaultDeployProject(env string) string {
	brandName := strings.ToLower(strings.TrimSpace(os.Getenv("CLI_CMD")))
	if brandName == "" {
		brandName = meta.Slug
	}
	envName := strings.ToLower(strings.TrimSpace(env))
	if envName == "" {
		envName = "default"
	}
	return fmt.Sprintf("%s-%s", brandName, envName)
}

func discoverRunningDeployTargetStacks() ([]deployTargetStack, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()
	containers, err := client.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	stacks := extractRunningDeployTargetStacks(containers)
	if len(stacks) == 0 {
		return nil, nil
	}
	return stacks, nil
}

func extractRunningDeployTargetStacks(containers []container.Summary) []deployTargetStack {
	if len(containers) == 0 {
		return nil
	}
	stacks := map[string]deployTargetStack{}
	priorities := map[string]int{}
	for _, ctr := range containers {
		if ctr.Labels == nil {
			continue
		}
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		priority, allowed := deployStackDiscoveryServicePriority[service]
		if !allowed {
			continue
		}
		stackName := inferStackFromServiceName(containerName(ctr.Names), service)
		if stackName == "" {
			continue
		}
		project := strings.TrimSpace(ctr.Labels[compose.ComposeProjectLabel])
		entry := deployTargetStack{
			Name:    stackName,
			Project: project,
			Env:     inferEnvFromStackName(stackName),
		}
		existing, ok := stacks[stackName]
		if !ok {
			stacks[stackName] = entry
			priorities[stackName] = priority
			continue
		}
		if priority < priorities[stackName] {
			if entry.Project == "" {
				entry.Project = existing.Project
			}
			stacks[stackName] = entry
			priorities[stackName] = priority
			continue
		}
		if existing.Project == "" && entry.Project != "" {
			existing.Project = entry.Project
			stacks[stackName] = existing
		}
	}
	if len(stacks) == 0 {
		return nil
	}
	names := make([]string, 0, len(stacks))
	for name := range stacks {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]deployTargetStack, 0, len(names))
	for _, name := range names {
		out = append(out, stacks[name])
	}
	return out
}

func inferStackFromServiceName(name, service string) string {
	trimmed := strings.TrimSpace(name)
	serviceName := strings.TrimSpace(service)
	if trimmed == "" || serviceName == "" {
		return ""
	}
	suffix := "-" + serviceName
	if !strings.HasSuffix(trimmed, suffix) {
		return ""
	}
	stack := strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
	if stack == "" {
		return ""
	}
	return stack
}

func inferEnvFromStackName(stack string) string {
	trimmed := strings.TrimSpace(stack)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "-")
	if len(parts) < 2 {
		return ""
	}
	env := strings.TrimSpace(parts[len(parts)-1])
	if env == "" {
		return ""
	}
	return env
}

func containerName(names []string) string {
	for _, raw := range names {
		trimmed := strings.TrimSpace(strings.TrimPrefix(raw, "/"))
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func inferDeployModeFromProject(composeProject string) (string, string, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return "", "", nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return "", "", fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()

	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, trimmed))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return "", "", fmt.Errorf("list containers: %w", err)
	}
	infos := containerInfos(containers)
	if mode := runtimecfg.InferModeFromContainers(infos, true); mode != "" {
		return mode, "running_services", nil
	}
	if mode := runtimecfg.InferModeFromContainers(infos, false); mode != "" {
		return mode, "services", nil
	}

	result, err := compose.ResolveComposeFilesFromProject(ctx, client, trimmed)
	if err == nil {
		if mode := runtimecfg.InferModeFromComposeFiles(result.Files); mode != "" {
			return mode, "config_files", nil
		}
	} else {
		return "", "", fmt.Errorf("resolve compose files: %w", err)
	}
	return "", "", nil
}

func containerInfos(containers []container.Summary) []runtimecfg.ContainerInfo {
	if len(containers) == 0 {
		return nil
	}
	out := make([]runtimecfg.ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		service := ""
		if ctr.Labels != nil {
			service = strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		}
		out = append(out, runtimecfg.ContainerInfo{
			Service: service,
			State:   strings.TrimSpace(ctr.State),
		})
	}
	return out
}
