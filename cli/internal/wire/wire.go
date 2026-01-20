// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Centralize CLI dependency construction for reuse by main and tests.
package wire

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
	// Getwd returns the current working directory. Tests may override this helper.
	Getwd = os.Getwd
	// NewDockerClient creates the Docker client. Tests may override this helper.
	NewDockerClient = compose.NewDockerClient
	// Stdout is the writer used for CLI output (used by app.Dependencies).
	Stdout = os.Stdout
)

// BuildDependencies constructs CLI dependencies. It returns the dependencies
// bundle, a closer for cleanup, and any initialization error.
func BuildDependencies() (app.Dependencies, io.Closer, error) {
	projectDir, err := Getwd()
	if err != nil {
		return app.Dependencies{}, nil, err
	}

	client, err := NewDockerClient()
	if err != nil {
		return app.Dependencies{}, nil, err
	}

	portDiscoverer := app.NewPortDiscoverer()
	builder := generator.NewGoBuilder(portDiscoverer)
	deps := app.Dependencies{
		ProjectDir:      projectDir,
		Out:             Stdout,
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

func warnf(_ string) {
	// Silently drop warnings to keep CLI output stable.
}

func asCloser(client compose.DockerClient) io.Closer {
	if closer, ok := client.(io.Closer); ok {
		return closer
	}
	return nil
}
