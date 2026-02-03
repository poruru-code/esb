// Where: cli/internal/infra/compose/client.go
// What: Docker client constructor.
// Why: Centralize Docker SDK initialization.
package compose

import (
	"fmt"

	"github.com/docker/docker/client"
)

// NewDockerClient constructs a Docker SDK client using environment defaults.
func NewDockerClient() (DockerClient, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return dockerClient, nil
}
