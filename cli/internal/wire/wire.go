// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Keep the build-only CLI dependencies scoped for reuse by main and tests.
package wire

import (
	"context"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/commands"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
)

// Stdout is the writer used for CLI output (used by app.Dependencies).
var Stdout = os.Stdout

type composePortDiscoverer struct{}

func (composePortDiscoverer) Discover(ctx context.Context, rootDir, project, mode string) (map[string]int, error) {
	return compose.DiscoverPorts(ctx, compose.ExecRunner{}, compose.PortDiscoveryOptions{
		RootDir: rootDir,
		Project: project,
		Mode:    mode,
	})
}

// BuildDependencies constructs CLI dependencies. It returns the dependencies
// bundle, a closer for cleanup, and any initialization error.
func BuildDependencies(_ []string) (commands.Dependencies, io.Closer, error) {
	builder := generator.NewGoBuilder(composePortDiscoverer{})

	deps := commands.Dependencies{
		Out:          Stdout,
		Prompter:     interaction.HuhPrompter{},
		RepoResolver: config.ResolveRepoRoot,
		Build: commands.BuildDeps{
			Builder: builder,
		},
	}

	return deps, nil, nil
}
