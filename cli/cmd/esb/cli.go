// Where: cli/cmd/esb/cli.go
// What: CLI dependency wiring helpers.
// Why: Centralize construction for testability.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/provisioner"
)

var (
	getwd           = os.Getwd
	newDockerClient = compose.NewDockerClient
)

func buildDependencies() (app.Dependencies, io.Closer, error) {
	projectDir, err := getwd()
	if err != nil {
		return app.Dependencies{}, nil, err
	}

	client, err := newDockerClient()
	if err != nil {
		return app.Dependencies{}, nil, err
	}

	deps := app.Dependencies{
		ProjectDir:      projectDir,
		Out:             os.Stdout,
		DetectorFactory: app.NewDetectorFactory(client, warnf),
		Builder:         generator.NewGoBuilder(),
		Downer:          app.NewDowner(client),
		Upper:           app.NewUpper(),
		Stopper:         app.NewStopper(),
		Logger:          app.NewLogger(),
		PortDiscoverer:  app.NewPortDiscoverer(),
		Waiter:          app.NewGatewayWaiter(),
		Provisioner:     provisionerAdapter{runner: provisioner.New(client)},
		Pruner:          app.NewPruner(),
	}

	return deps, asCloser(client), nil
}

type provisionerAdapter struct {
	runner *provisioner.Runner
}

func (p provisionerAdapter) Provision(request app.ProvisionRequest) error {
	if p.runner == nil {
		return fmt.Errorf("provisioner is nil")
	}
	return p.runner.Provision(provisioner.Request{
		TemplatePath:   request.TemplatePath,
		ProjectDir:     request.ProjectDir,
		Env:            request.Env,
		ComposeProject: request.ComposeProject,
		Mode:           request.Mode,
	})
}

func warnf(message string) {
	fmt.Fprintln(os.Stderr, message)
}

func asCloser(client compose.DockerClient) io.Closer {
	if closer, ok := client.(io.Closer); ok {
		return closer
	}
	return nil
}
