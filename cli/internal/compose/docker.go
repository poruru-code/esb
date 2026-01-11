// Where: cli/internal/compose/docker.go
// What: Docker SDK helpers for containers and images.
// Why: Provide scoped queries for state detection.
package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
)

// DockerClient defines the subset of Docker SDK methods used by this package.
// This interface enables mocking the Docker client in tests.
type DockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
}

// ListContainersByProject returns container information for all containers
// belonging to the specified Docker Compose project.
func ListContainersByProject(
	ctx context.Context,
	client DockerClient,
	project string,
) ([]state.ContainerInfo, error) {
	labelFilter := filters.NewArgs()
	labelFilter.Add("label", fmt.Sprintf("%s=%s", composeProjectLabel, project))

	containers, err := client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: labelFilter,
	})
	if err != nil {
		return nil, err
	}

	result := make([]state.ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		if ctr.Labels == nil || ctr.Labels[composeProjectLabel] != project {
			continue
		}

		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		result = append(result, state.ContainerInfo{
			Name:    name,
			Service: ctr.Labels[composeServiceLabel],
			State:   ctr.State,
		})
	}
	return result, nil
}

// HasImagesForEnv checks if any Docker images exist with a tag suffix
// matching the specified environment name (e.g., ":prod").
func HasImagesForEnv(ctx context.Context, client DockerClient, env string) (bool, error) {
	images, err := client.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return false, err
	}

	needle := ":" + env
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == "<none>:<none>" {
				continue
			}
			if strings.HasSuffix(tag, needle) {
				return true, nil
			}
		}
	}
	return false, nil
}
