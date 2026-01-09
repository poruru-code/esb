// Where: cli/internal/app/deps.go
// What: Dependency wiring for detector usage.
// Why: Centralize detector construction with SDK helpers.
package app

import (
	"context"
	"errors"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

var ErrDockerClientNil = errors.New("docker client is nil")

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

// NewDowner creates a Downer implementation that uses the Docker client
// to stop and remove containers via Docker Compose.
func NewDowner(client compose.DockerClient) Downer {
	return downerFunc(func(project string, removeVolumes bool) error {
		return compose.DownProject(context.Background(), client, project, removeVolumes)
	})
}

// downerFunc is a function adapter that implements the Downer interface.
type downerFunc func(project string, removeVolumes bool) error

// Down implements the Downer interface by invoking the wrapped function.
func (fn downerFunc) Down(project string, removeVolumes bool) error {
	return fn(project, removeVolumes)
}

// NewUpper creates an Upper implementation that starts containers
// via Docker Compose with the specified options.
func NewUpper() Upper {
	return upperFunc(func(request UpRequest) error {
		rootDir, err := compose.FindRepoRoot(request.Context.ProjectDir)
		if err != nil {
			return err
		}

		opts := compose.UpOptions{
			RootDir: rootDir,
			Project: request.Context.ComposeProject,
			Target:  "control",
			Detach:  request.Detach,
		}
		return compose.UpProject(context.Background(), compose.ExecRunner{}, opts)
	})
}

// upperFunc is a function adapter that implements the Upper interface.
type upperFunc func(request UpRequest) error

// Up implements the Upper interface by invoking the wrapped function.
func (fn upperFunc) Up(request UpRequest) error {
	return fn(request)
}

// NewStopper creates a Stopper implementation that stops containers
// via Docker Compose without removing them.
func NewStopper() Stopper {
	return stopperFunc(func(request StopRequest) error {
		rootDir, err := compose.FindRepoRoot(request.Context.ProjectDir)
		if err != nil {
			return err
		}

		opts := compose.StopOptions{
			RootDir: rootDir,
			Project: request.Context.ComposeProject,
			Mode:    request.Context.Mode,
			Target:  "control",
		}
		return compose.StopProject(context.Background(), compose.ExecRunner{}, opts)
	})
}

// stopperFunc is a function adapter that implements the Stopper interface.
type stopperFunc func(request StopRequest) error

// Stop implements the Stopper interface by invoking the wrapped function.
func (fn stopperFunc) Stop(request StopRequest) error {
	return fn(request)
}

// NewLogger creates a Logger implementation that streams container logs
// via Docker Compose with follow/tail/timestamp options.
func NewLogger() Logger {
	return loggerFunc(func(request LogsRequest) error {
		rootDir, err := compose.FindRepoRoot(request.Context.ProjectDir)
		if err != nil {
			return err
		}

		opts := compose.LogsOptions{
			RootDir:    rootDir,
			Project:    request.Context.ComposeProject,
			Mode:       request.Context.Mode,
			Target:     "control",
			Follow:     request.Follow,
			Tail:       request.Tail,
			Timestamps: request.Timestamps,
			Service:    request.Service,
		}
		return compose.LogsProject(context.Background(), compose.ExecRunner{}, opts)
	})
}

// loggerFunc is a function adapter that implements the Logger interface.
type loggerFunc func(request LogsRequest) error

// Logs implements the Logger interface by invoking the wrapped function.
func (fn loggerFunc) Logs(request LogsRequest) error {
	return fn(request)
}

// NewPruner creates a Pruner implementation that removes generated artifacts
// and optionally the generator.yml configuration file.
func NewPruner() Pruner {
	return prunerFunc(func(request PruneRequest) error {
		if err := os.RemoveAll(request.Context.OutputEnvDir); err != nil {
			return err
		}
		if request.Hard {
			if err := os.Remove(request.Context.GeneratorPath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

// prunerFunc is a function adapter that implements the Pruner interface.
type prunerFunc func(request PruneRequest) error

// Prune implements the Pruner interface by invoking the wrapped function.
func (fn prunerFunc) Prune(request PruneRequest) error {
	return fn(request)
}
