// Where: cli/internal/compose/down.go
// What: Down helpers using Docker SDK.
// Why: Stop and remove containers for a compose project.
package compose

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// DownProject stops and removes all containers belonging to the specified
// Docker Compose project. Optionally removes volumes if removeVolumes is true.
func DownProject(ctx context.Context, client DockerClient, project string, removeVolumes bool) error {
	labelFilter := filters.NewArgs()
	labelFilter.Add("label", fmt.Sprintf("%s=%s", ComposeProjectLabel, project))

	containers, err := client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: labelFilter,
	})
	if err != nil {
		return err
	}

	for _, ctr := range containers {
		if ctr.Labels == nil || ctr.Labels[ComposeProjectLabel] != project {
			continue
		}
		if ctr.State == "running" {
			if err := client.ContainerStop(ctx, ctr.ID, container.StopOptions{}); err != nil {
				return err
			}
		}
		if err := client.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{RemoveVolumes: removeVolumes}); err != nil {
			return err
		}
	}

	return nil
}
