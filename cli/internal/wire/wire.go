// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Centralize CLI dependency construction for reuse by main and tests.
package wire

import (
	"io"
	"os"
	"sync"

	"github.com/poruru/edge-serverless-box/cli/internal/commands"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
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
func BuildDependencies(args []string) (commands.Dependencies, io.Closer, error) {
	projectDir, err := Getwd()
	if err != nil {
		return commands.Dependencies{}, nil, err
	}

	var provider *dockerClientProvider
	if requiresDockerClient(args) {
		provider = &dockerClientProvider{factory: NewDockerClient}
	}

	portDiscoverer := helpers.NewPortDiscoverer()
	builder := generator.NewGoBuilder(portDiscoverer)
	var detectorFactory helpers.DetectorFactory
	var dockerFactory helpers.DockerClientFactory
	if provider != nil {
		dockerFactory = provider.Get
		detectorFactory = lazyDetectorFactory(dockerFactory)
	}

	deps := commands.Dependencies{
		ProjectDir:          projectDir,
		Out:                 Stdout,
		DetectorFactory:     detectorFactory,
		DockerClientFactory: dockerFactory,
		Prompter:            interaction.HuhPrompter{},
		RepoResolver:        config.ResolveRepoRoot,
		GlobalConfigLoader:  helpers.DefaultGlobalConfigLoader(),
		ProjectConfigLoader: helpers.DefaultProjectConfigLoader(),
		ProjectDirFinder:    helpers.DefaultProjectDirFinder(),
		Build: commands.BuildDeps{
			Builder: builder,
		},
	}

	var closer io.Closer
	if provider != nil {
		closer = provider
	}
	return deps, closer, nil
}

func warnf(_ string) {
	// Silently drop warnings to keep CLI output stable.
}

type dockerClientProvider struct {
	once    sync.Once
	factory helpers.DockerClientFactory
	client  compose.DockerClient
	err     error
}

func (p *dockerClientProvider) Get() (compose.DockerClient, error) {
	p.once.Do(func() {
		p.client, p.err = p.factory()
	})
	return p.client, p.err
}

func (p *dockerClientProvider) Close() error {
	if closer, ok := p.client.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func lazyDetectorFactory(factory helpers.DockerClientFactory) helpers.DetectorFactory {
	return func(projectDir, env string) (ports.StateDetector, error) {
		client, err := factory()
		if err != nil {
			return nil, err
		}
		return helpers.NewDetectorFactory(client, warnf)(projectDir, env)
	}
}

func requiresDockerClient(args []string) bool {
	// Currently only build might need it for image existence checks
	// or other internal logic, but we keep it for now.
	switch commands.CommandName(args) {
	case "build":
		return true
	default:
		return false
	}
}
