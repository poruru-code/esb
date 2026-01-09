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

func NewDowner(client compose.DockerClient) Downer {
	return downerFunc(func(project string, removeVolumes bool) error {
		return compose.DownProject(context.Background(), client, project, removeVolumes)
	})
}

type downerFunc func(project string, removeVolumes bool) error

func (fn downerFunc) Down(project string, removeVolumes bool) error {
	return fn(project, removeVolumes)
}

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

type upperFunc func(request UpRequest) error

func (fn upperFunc) Up(request UpRequest) error {
	return fn(request)
}

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

type stopperFunc func(request StopRequest) error

func (fn stopperFunc) Stop(request StopRequest) error {
	return fn(request)
}

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

type loggerFunc func(request LogsRequest) error

func (fn loggerFunc) Logs(request LogsRequest) error {
	return fn(request)
}

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

type prunerFunc func(request PruneRequest) error

func (fn prunerFunc) Prune(request PruneRequest) error {
	return fn(request)
}
