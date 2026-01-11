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

// buildDependencies constructs all runtime dependencies required by the CLI.
// It initializes the Docker client, generator, and various command handlers.
// Returns the dependencies, a closer for cleanup, and any initialization error.
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
		Prompter:        app.HuhPrompter{},
	}

	return deps, asCloser(client), nil
}

// provisionerAdapter wraps the provisioner.Runner to implement the app.Provisioner interface.
// This adapter translates between the application-level ProvisionRequest and
// the lower-level provisioner.Request.
type provisionerAdapter struct {
	runner *provisioner.Runner
}

// Provision executes the provisioning workflow by delegating to the underlying runner.
// It converts the application-level request to a provisioner-specific request and
// returns any error encountered during the provisioning process.
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

// warnf writes a warning message to stderr.
// Used as a callback for the detector factory to report non-fatal issues.
func warnf(_ string) {
	// Silenced to prevent stderr output from disrupting CLI layout.
	// fmt.Fprintln(os.Stderr, message)
}

// asCloser attempts to cast the Docker client to an io.Closer.
// Returns nil if the client does not implement the Closer interface.
func asCloser(client compose.DockerClient) io.Closer {
	if closer, ok := client.(io.Closer); ok {
		return closer
	}
	return nil
}
