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

	"github.com/poruru-code/esb/cli/internal/command"
	"github.com/poruru-code/esb/cli/internal/domain/state"
	"github.com/poruru-code/esb/cli/internal/infra/build"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
	"github.com/poruru-code/esb/cli/internal/infra/config"
	infradeploy "github.com/poruru-code/esb/cli/internal/infra/deploy"
	"github.com/poruru-code/esb/cli/internal/infra/env"
	"github.com/poruru-code/esb/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru-code/esb/cli/internal/infra/runtime"
	"github.com/poruru-code/esb/cli/internal/infra/ui"
)

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
	composeRunner := compose.ExecRunner{}

	deps := command.Dependencies{
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		Prompter:     interaction.HuhPrompter{},
		RepoResolver: config.ResolveRepoRoot,
		Deploy: command.DeployDeps{
			Build:     newDeployBuildDeps(builder),
			Runtime:   newDeployRuntimeDeps(config.ResolveRepoRoot),
			Provision: newDeployProvisionDeps(composeRunner),
		},
	}

	return deps, nil, nil
}

func newDeployBuildDeps(builder *build.GoBuilder) command.DeployBuildDeps {
	return command.DeployBuildDeps{
		Build: builder.Build,
	}
}

func newDeployRuntimeDeps(repoResolver func(string) (string, error)) command.DeployRuntimeDeps {
	return command.DeployRuntimeDeps{
		ApplyRuntimeEnv: func(ctx state.Context) error {
			return env.ApplyRuntimeEnv(ctx, repoResolver)
		},
		RuntimeEnvResolver: runtimeinfra.NewEnvResolver(),
		DockerClient:       compose.NewDockerClient,
	}
}

func newDeployProvisionDeps(composeRunner compose.CommandRunner) command.DeployProvisionDeps {
	deployUIFactory := func(out io.Writer, emojiEnabled bool) ui.UserInterface {
		return ui.NewDeployUI(out, emojiEnabled)
	}
	composeProvisionerFactory := func(u ui.UserInterface) command.ComposeProvisioner {
		return infradeploy.NewComposeProvisioner(composeRunner, u)
	}
	return command.DeployProvisionDeps{
		ComposeRunner:             composeRunner,
		ComposeProvisionerFactory: composeProvisionerFactory,
		NewDeployUI:               deployUIFactory,
	}
}
