// Where: cli/internal/compose/client.go
// What: Docker client constructor.
// Why: Centralize Docker SDK initialization.
package compose

import "github.com/docker/docker/client"

// NewDockerClient constructs a Docker SDK client using environment defaults.
func NewDockerClient() (DockerClient, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}
