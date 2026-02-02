// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Keep the build-only CLI dependencies scoped for reuse by main and tests.
package wire

import (
	"context"
	"io"
	"os"
	"path/filepath"

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
	extraFiles := []string{}
	infraFile := filepath.Join(rootDir, "docker-compose.infra.yml")
	if _, err := os.Stat(infraFile); err == nil {
		extraFiles = append(extraFiles, infraFile)
	}
	return compose.DiscoverPorts(ctx, compose.ExecRunner{}, compose.PortDiscoveryOptions{
		RootDir:    rootDir,
		Project:    project,
		Mode:       mode,
		ExtraFiles: extraFiles,
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
		Deploy: commands.DeployDeps{
			Builder: builder,
		},
	}

	return deps, nil, nil
}
