// Where: cli/internal/helpers/docker_client.go
// What: Docker client factory definition.
// Why: Provide a shared type for lazy Docker client initialization.
package helpers

import "github.com/poruru/edge-serverless-box/cli/internal/compose"

// DockerClientFactory returns a Docker client for CLI helpers.
type DockerClientFactory func() (compose.DockerClient, error)
