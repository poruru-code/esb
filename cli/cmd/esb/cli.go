// Where: cli/cmd/esb/cli.go
// What: CLI dependency wiring helpers.
// Why: Centralize construction for testability.
package main

import (
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
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
		Upper:           app.NewUpper(config.ResolveRepoRoot),
		Stopper:         app.NewStopper(config.ResolveRepoRoot),
		Logger:          app.NewLogger(client, config.ResolveRepoRoot),
		PortDiscoverer:  app.NewPortDiscoverer(),
		Waiter:          app.NewGatewayWaiter(),
		Provisioner:     provisioner.New(client),
		Parser:          generator.DefaultParser{},
		Pruner:          app.NewPruner(client),
		Prompter:        app.HuhPrompter{},
		RepoResolver:    config.ResolveRepoRoot,
	}

	return deps, asCloser(client), nil
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
