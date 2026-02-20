// Where: cli/internal/usecase/deploy/runtime_observation.go
// What: Runtime observation for artifact compatibility checks.
// Why: Feed live/fallback runtime facts into artifactcore runtime_stack validation.
package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
	"github.com/poruru/edge-serverless-box/pkg/runtimeimage"
)

func (w Workflow) resolveRuntimeObservation(req Request) (*artifactcore.RuntimeObservation, []string) {
	observation := &artifactcore.RuntimeObservation{
		Mode:       strings.TrimSpace(req.Context.Mode),
		ESBVersion: strings.TrimSpace(req.Tag),
		Source:     "deploy request",
	}
	warnings := make([]string, 0)

	project := strings.TrimSpace(req.Context.ComposeProject)
	if project == "" || w.DockerClient == nil {
		return observation, warnings
	}
	client, err := w.newDockerClient()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("resolve runtime observation docker client: %v", err))
		return observation, warnings
	}

	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, project))
	containers, err := client.ContainerList(context.Background(), container.ListOptions{
		All:     false,
		Filters: filterArgs,
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("resolve runtime observation container list: %v", err))
		return observation, warnings
	}
	serviceImages := selectServiceImages(containers)
	if len(serviceImages) == 0 {
		warnings = append(warnings, fmt.Sprintf("runtime observation found no compose services for project %q", project))
		return observation, warnings
	}

	imageRef, service := runtimeimage.PreferredServiceImage(serviceImages)
	if mode := runtimeimage.InferModeFromServiceImages(serviceImages); mode != "" {
		observation.Mode = mode
	}
	if tag := runtimeimage.ParseTag(imageRef); tag != "" {
		observation.ESBVersion = tag
	}
	observation.Source = fmt.Sprintf("docker compose project=%s service=%s", project, service)
	return observation, warnings
}

func selectServiceImages(containers []container.Summary) map[string]string {
	images := make(map[string]string)
	for _, ctr := range containers {
		state := strings.ToLower(strings.TrimSpace(ctr.State))
		if state != "running" {
			continue
		}
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		if service == "" {
			continue
		}
		imageRef := strings.TrimSpace(ctr.Image)
		if imageRef == "" {
			continue
		}
		images[service] = imageRef
	}
	return images
}
