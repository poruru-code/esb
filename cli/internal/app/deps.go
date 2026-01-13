// Where: cli/internal/app/deps.go
// What: Dependency wiring for detector usage.
// Why: Centralize detector construction with SDK helpers.
package app

import (
	"context"
	"errors"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
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
func NewUpper(resolver func(string) (string, error)) Upper {
	return upperFunc(func(request UpRequest) error {
		if resolver == nil {
			resolver = config.ResolveRepoRoot
		}
		rootDir, err := resolver(request.Context.ProjectDir)
		if err != nil {
			return err
		}

		opts := compose.UpOptions{
			RootDir: rootDir,
			Project: request.Context.ComposeProject,
			Target:  "control",
			Detach:  request.Detach,
			EnvFile: request.EnvFile,
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
func NewStopper(resolver func(string) (string, error)) Stopper {
	return stopperFunc(func(request StopRequest) error {
		if resolver == nil {
			resolver = config.ResolveRepoRoot
		}
		rootDir, err := resolver(request.Context.ProjectDir)
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

// Logger defines the interface for streaming container logs and listing services.
// Implementations use Docker Compose to retrieve log output and configuration.
type Logger interface {
	Logs(request LogsRequest) error
	ListServices(request LogsRequest) ([]string, error)
	ListContainers(project string) ([]state.ContainerInfo, error)
}

// NewLogger creates a Logger implementation that streams container logs
// via Docker Compose with follow/tail/timestamp options.
func NewLogger(client compose.DockerClient, resolver func(string) (string, error)) Logger {
	if resolver == nil {
		resolver = config.ResolveRepoRoot
	}
	return loggerImpl{
		logsFn: func(request LogsRequest) error {
			rootDir, err := resolver(request.Context.ProjectDir)
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
		},
		listServicesFn: func(request LogsRequest) ([]string, error) {
			rootDir, err := resolver(request.Context.ProjectDir)
			if err != nil {
				return nil, err
			}

			opts := compose.LogsOptions{
				RootDir: rootDir,
				Project: request.Context.ComposeProject,
				Mode:    request.Context.Mode,
				Target:  "control",
			}
			return compose.ListServices(context.Background(), compose.ExecRunner{}, opts)
		},
		listContainersFn: func(project string) ([]state.ContainerInfo, error) {
			if client == nil {
				return nil, ErrDockerClientNil
			}
			return compose.ListContainersByProject(context.Background(), client, project)
		},
	}
}

// loggerImpl implements the Logger interface using function adapters.
type loggerImpl struct {
	logsFn           func(request LogsRequest) error
	listServicesFn   func(request LogsRequest) ([]string, error)
	listContainersFn func(project string) ([]state.ContainerInfo, error)
}

// Logs implements the Logger interface by invoking the wrapped function.
func (l loggerImpl) Logs(request LogsRequest) error {
	return l.logsFn(request)
}

// ListServices implements the Logger interface by invoking the wrapped function.
func (l loggerImpl) ListServices(request LogsRequest) ([]string, error) {
	return l.listServicesFn(request)
}

// ListContainers implements the Logger interface by invoking the wrapped function.
func (l loggerImpl) ListContainers(project string) ([]state.ContainerInfo, error) {
	return l.listContainersFn(project)
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
