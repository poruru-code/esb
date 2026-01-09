// Where: cli/internal/provisioner/ports.go
// What: Port resolution helpers for local services.
// Why: Discover dynamic ports when Docker Compose assigns them.
package provisioner

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
)

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
)

type PortRequest struct {
	Project       string
	Service       string
	ContainerPort int
}

type PortResolver interface {
	Resolve(ctx context.Context, request PortRequest) (int, error)
}

type dockerPortResolver struct {
	Client compose.DockerClient
}

func (r dockerPortResolver) Resolve(ctx context.Context, request PortRequest) (int, error) {
	if r.Client == nil {
		return 0, fmt.Errorf("docker client is nil")
	}
	if strings.TrimSpace(request.Project) == "" {
		return 0, fmt.Errorf("compose project is required")
	}
	if strings.TrimSpace(request.Service) == "" {
		return 0, fmt.Errorf("compose service is required")
	}
	if request.ContainerPort <= 0 {
		return 0, fmt.Errorf("container port is required")
	}

	labelFilter := filters.NewArgs()
	labelFilter.Add("label", fmt.Sprintf("%s=%s", composeProjectLabel, request.Project))

	containers, err := r.Client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: labelFilter,
	})
	if err != nil {
		return 0, err
	}

	for _, ctr := range containers {
		if ctr.Labels == nil || ctr.Labels[composeProjectLabel] != request.Project {
			continue
		}
		if ctr.Labels[composeServiceLabel] != request.Service {
			continue
		}
		for _, port := range ctr.Ports {
			if int(port.PrivatePort) != request.ContainerPort {
				continue
			}
			if port.PublicPort > 0 {
				return int(port.PublicPort), nil
			}
		}
	}

	return 0, fmt.Errorf("published port not found for %s:%d", request.Service, request.ContainerPort)
}

func resolvePort(
	ctx context.Context,
	envVar string,
	defaultPort int,
	request PortRequest,
	resolver PortResolver,
) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw != "" {
		if port, err := strconv.Atoi(raw); err == nil {
			if port > 0 {
				return port, true
			}
			if resolver != nil {
				if resolved, err := resolver.Resolve(ctx, request); err == nil && resolved > 0 {
					return resolved, true
				}
			}
			return 0, false
		}
	}

	if resolver != nil {
		if resolved, err := resolver.Resolve(ctx, request); err == nil && resolved > 0 {
			return resolved, true
		}
	}
	if defaultPort > 0 {
		return defaultPort, true
	}
	return 0, false
}
