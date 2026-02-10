// Where: cli/internal/app/di.go
// What: CLI dependency wiring.
// Why: Keep the build-only CLI dependencies scoped for reuse by main and tests.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/command"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
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
	composeFiles := []string{}
	if strings.TrimSpace(project) != "" {
		if client, err := compose.NewDockerClient(); err == nil {
			result, err := compose.ResolveComposeFilesFromProject(ctx, client, project)
			if err == nil && len(result.Files) > 0 {
				composeFiles = result.Files
				extraFiles = nil
			}
		}
	}
	ports, err := compose.DiscoverPorts(ctx, compose.ExecRunner{}, compose.PortDiscoveryOptions{
		RootDir:      rootDir,
		Project:      project,
		Mode:         mode,
		ExtraFiles:   extraFiles,
		ComposeFiles: composeFiles,
	})
	if err != nil {
		return nil, fmt.Errorf("discover ports: %w", err)
	}
	return ports, nil
}

// BuildDependencies constructs CLI dependencies. It returns the dependencies
// bundle, a closer for cleanup, and any initialization error.
func BuildDependencies(_ []string) (command.Dependencies, io.Closer, error) {
	builder := build.NewGoBuilder(composePortDiscoverer{})

	deps := command.Dependencies{
		Out:          Stdout,
		Prompter:     interaction.HuhPrompter{},
		RepoResolver: config.ResolveRepoRoot,
		Deploy: command.DeployDeps{
			Build:              builder.Build,
			RuntimeEnvResolver: runtimeinfra.NewEnvResolver(),
		},
	}

	return deps, nil, nil
}
