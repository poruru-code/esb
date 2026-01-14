// Where: cli/internal/compose/prune.go
// What: ESB-scoped Docker prune helpers.
// Why: Provide a system-prune-like cleanup limited to an ESB compose project.
package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// PruneOptions configures ESB-scoped cleanup behavior.
type PruneOptions struct {
	Project       string
	RemoveVolumes bool
	AllImages     bool
}

// PruneReport summarizes what was deleted during prune.
type PruneReport struct {
	ContainersDeleted []string
	NetworksDeleted   []string
	VolumesDeleted    []string
	ImagesDeleted     []image.DeleteResponse
	SpaceReclaimed    uint64
}

// PruneProject deletes ESB resources scoped to a compose project label.
// It removes stopped containers, unused networks, dangling/unused images,
// and optionally volumes. Image pruning is limited to ESB-labeled images.
func PruneProject(ctx context.Context, client DockerClient, opts PruneOptions) (PruneReport, error) {
	if client == nil {
		return PruneReport{}, fmt.Errorf("docker client is nil")
	}
	project := strings.TrimSpace(opts.Project)
	if project == "" {
		return PruneReport{}, fmt.Errorf("compose project is required")
	}

	report := PruneReport{}
	projectFilter := filters.NewArgs(filters.Arg("label", fmt.Sprintf("%s=%s", ComposeProjectLabel, project)))

	containers, err := client.ContainersPrune(ctx, projectFilter)
	if err != nil {
		return report, err
	}
	report.ContainersDeleted = append(report.ContainersDeleted, containers.ContainersDeleted...)
	report.SpaceReclaimed += containers.SpaceReclaimed

	networks, err := client.NetworksPrune(ctx, projectFilter)
	if err != nil {
		return report, err
	}
	report.NetworksDeleted = append(report.NetworksDeleted, networks.NetworksDeleted...)

	if opts.RemoveVolumes {
		volumes, err := client.VolumesPrune(ctx, projectFilter)
		if err != nil {
			return report, err
		}
		report.VolumesDeleted = append(report.VolumesDeleted, volumes.VolumesDeleted...)
		report.SpaceReclaimed += volumes.SpaceReclaimed
	}

	images, err := pruneImages(ctx, client, project, opts.AllImages)
	if err != nil {
		return report, err
	}
	report.ImagesDeleted = append(report.ImagesDeleted, images.ImagesDeleted...)
	report.SpaceReclaimed += images.SpaceReclaimed

	return report, nil
}

func pruneImages(ctx context.Context, client DockerClient, project string, all bool) (image.PruneReport, error) {
	report := image.PruneReport{}
	for _, label := range []string{ComposeProjectLabel, ESBProjectLabel} {
		pruneFilters := imagePruneFilters(label, project, all)
		pruned, err := client.ImagesPrune(ctx, pruneFilters)
		if err != nil {
			return report, err
		}
		report.ImagesDeleted = append(report.ImagesDeleted, pruned.ImagesDeleted...)
		report.SpaceReclaimed += pruned.SpaceReclaimed
	}
	return report, nil
}

func imagePruneFilters(labelKey, project string, all bool) filters.Args {
	pruneFilters := filters.NewArgs(filters.Arg("label", fmt.Sprintf("%s=%s", labelKey, project)))
	if labelKey == ESBProjectLabel {
		pruneFilters.Add("label", fmt.Sprintf("%s=true", ESBManagedLabel))
	}
	if all {
		pruneFilters.Add("dangling", "false")
	} else {
		pruneFilters.Add("dangling", "true")
	}
	return pruneFilters
}
