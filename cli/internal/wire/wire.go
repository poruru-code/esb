// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Centralize CLI dependency construction for reuse by main and tests.
package wire

import (
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/commands"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
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
func BuildDependencies() (commands.Dependencies, io.Closer, error) {
	projectDir, err := Getwd()
	if err != nil {
		return commands.Dependencies{}, nil, err
	}

	client, err := NewDockerClient()
	if err != nil {
		return commands.Dependencies{}, nil, err
	}

	portDiscoverer := helpers.NewPortDiscoverer()
	builder := generator.NewGoBuilder(portDiscoverer)
	deps := commands.Dependencies{
		ProjectDir:      projectDir,
		Out:             Stdout,
		DetectorFactory: helpers.NewDetectorFactory(client, warnf),
		Prompter:        commands.HuhPrompter{},
		RepoResolver:    config.ResolveRepoRoot,
		Build: commands.BuildDeps{
			Builder: builder,
		},
		Up: commands.UpDeps{
			Builder:        builder,
			Upper:          helpers.NewUpper(config.ResolveRepoRoot),
			Downer:         helpers.NewDowner(client),
			PortDiscoverer: portDiscoverer,
			Waiter:         helpers.NewGatewayWaiter(),
			Provisioner:    provisioner.New(client),
			Parser:         generator.DefaultParser{},
		},
		Down: commands.DownDeps{
			Downer: helpers.NewDowner(client),
		},
		Logs: commands.LogsDeps{
			Logger: helpers.NewLogger(client, config.ResolveRepoRoot),
		},
		Stop: commands.StopDeps{
			Stopper: helpers.NewStopper(config.ResolveRepoRoot),
		},
		Prune: commands.PruneDeps{
			Pruner: helpers.NewPruner(client),
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
