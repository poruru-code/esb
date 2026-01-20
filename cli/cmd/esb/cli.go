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

	portDiscoverer := app.NewPortDiscoverer()
	builder := generator.NewGoBuilder(portDiscoverer)
	deps := app.Dependencies{
		ProjectDir:      projectDir,
		Out:             os.Stdout,
		DetectorFactory: app.NewDetectorFactory(client, warnf),
		Prompter:        app.HuhPrompter{},
		RepoResolver:    config.ResolveRepoRoot,
		Build: app.BuildDeps{
			Builder: builder,
		},
		Up: app.UpDeps{
			Builder:        builder,
			Upper:          app.NewUpper(config.ResolveRepoRoot),
			Downer:         app.NewDowner(client),
			PortDiscoverer: portDiscoverer,
			Waiter:         app.NewGatewayWaiter(),
			Provisioner:    provisioner.New(client),
			Parser:         generator.DefaultParser{},
		},
		Down: app.DownDeps{
			Downer: app.NewDowner(client),
		},
		Logs: app.LogsDeps{
			Logger: app.NewLogger(client, config.ResolveRepoRoot),
		},
		Stop: app.StopDeps{
			Stopper: app.NewStopper(config.ResolveRepoRoot),
		},
		Prune: app.PruneDeps{
			Pruner: app.NewPruner(client),
		},
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
