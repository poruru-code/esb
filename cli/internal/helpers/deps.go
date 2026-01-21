// Where: cli/internal/helpers/deps.go
// What: Dependency wiring for detector usage.
// Why: Centralize detector construction with SDK helpers.
package helpers

import (
	"context"
	"errors"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

var ErrDockerClientNil = errors.New("docker client is nil")

type (
	StateDetector   = ports.StateDetector
	DetectorFactory = ports.DetectorFactory
)

// NewPortDiscoverer returns a PortDiscoverer implementation that uses
// docker compose to find dynamic host ports.
func NewPortDiscoverer() generator.PortDiscoverer {
	return portDiscovererAdapter{}
}

type portDiscovererAdapter struct{}

func (p portDiscovererAdapter) Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error) {
	return compose.DiscoverPorts(ctx, compose.ExecRunner{}, compose.PortDiscoveryOptions{
		RootDir: rootDir,
		Project: project,
		Mode:    mode,
	})
}

// NewDetectorFactory creates a DetectorFactory that constructs StateDetectors
// using the provided Docker client. The warn function is called for non-fatal issues.
func NewDetectorFactory(client compose.DockerClient, warn func(string)) DetectorFactory {
	return func(projectDir, env string) (StateDetector, error) {
		if client == nil {
			return nil, ErrDockerClientNil
		}

		detector := state.Detector{
			ProjectDir: projectDir,
			Env:        env,
			ResolveContext: func(projectDir, env string) (state.Context, error) {
				return state.ResolveContext(projectDir, env)
			},
			ListContainers: func(project string) ([]state.ContainerInfo, error) {
				return compose.ListContainersByProject(context.Background(), client, project)
			},
			HasBuildArtifacts: state.HasBuildArtifacts,
			HasImages: func(ctx state.Context) (bool, error) {
				return compose.HasImagesForEnv(context.Background(), client, ctx.Env)
			},
			Warn: warn,
		}

		return detector, nil
	}
}
