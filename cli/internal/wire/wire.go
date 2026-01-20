// Where: cli/internal/wire/wire.go
// What: CLI dependency wiring.
// Why: Centralize CLI dependency construction for reuse by main and tests.
package wire

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/poruru/edge-serverless-box/cli/internal/commands"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/provisioner"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
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
	portStateStore := helpers.NewPortStateStore()
	builder := generator.NewGoBuilder(portDiscoverer)
	var detectorFactory helpers.DetectorFactory
	var dockerFactory helpers.DockerClientFactory
	var downer ports.Downer
	var logger ports.Logger
	var pruner ports.Pruner
	var provisionerSvc ports.Provisioner
	if provider != nil {
		dockerFactory = provider.Get
		detectorFactory = lazyDetectorFactory(dockerFactory)
		downer = lazyDowner{factory: dockerFactory}
		logger = lazyLogger{factory: dockerFactory, resolver: config.ResolveRepoRoot}
		pruner = lazyPruner{factory: dockerFactory}
		provisionerSvc = lazyProvisioner{factory: dockerFactory}
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
		Up: commands.UpDeps{
			Builder:        builder,
			Upper:          helpers.NewUpper(config.ResolveRepoRoot),
			Downer:         downer,
			PortDiscoverer: portDiscoverer,
			PortStateStore: portStateStore,
			Waiter:         helpers.NewGatewayWaiter(),
			Provisioner:    provisionerSvc,
			Parser:         generator.DefaultParser{},
		},
		Down: commands.DownDeps{
			Downer: downer,
		},
		Logs: commands.LogsDeps{
			Logger: logger,
		},
		Stop: commands.StopDeps{
			Stopper: helpers.NewStopper(config.ResolveRepoRoot),
		},
		Prune: commands.PruneDeps{
			Pruner: pruner,
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

type lazyDowner struct {
	factory helpers.DockerClientFactory
}

func (l lazyDowner) Down(project string, removeVolumes bool) error {
	client, err := l.factory()
	if err != nil {
		return err
	}
	return helpers.NewDowner(client).Down(project, removeVolumes)
}

type lazyLogger struct {
	factory  helpers.DockerClientFactory
	resolver func(string) (string, error)
}

func (l lazyLogger) Logs(request ports.LogsRequest) error {
	client, err := l.factory()
	if err != nil {
		return err
	}
	return helpers.NewLogger(client, l.resolver).Logs(request)
}

func (l lazyLogger) ListServices(request ports.LogsRequest) ([]string, error) {
	client, err := l.factory()
	if err != nil {
		return nil, err
	}
	return helpers.NewLogger(client, l.resolver).ListServices(request)
}

func (l lazyLogger) ListContainers(project string) ([]state.ContainerInfo, error) {
	client, err := l.factory()
	if err != nil {
		return nil, err
	}
	return helpers.NewLogger(client, l.resolver).ListContainers(project)
}

type lazyPruner struct {
	factory helpers.DockerClientFactory
}

func (l lazyPruner) Prune(request ports.PruneRequest) error {
	client, err := l.factory()
	if err != nil {
		return err
	}
	return helpers.NewPruner(client).Prune(request)
}

type lazyProvisioner struct {
	factory helpers.DockerClientFactory
}

func (l lazyProvisioner) Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error {
	client, err := l.factory()
	if err != nil {
		return err
	}
	return provisioner.New(client).Apply(ctx, resources, composeProject)
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
	switch commands.CommandName(args) {
	case "up", "down", "logs", "stop", "prune":
		return true
	case "env":
		return envCommandNeedsDocker(args)
	default:
		return false
	}
}

func envCommandNeedsDocker(args []string) bool {
	for i, arg := range args {
		if arg != "env" {
			continue
		}
		return nextCommandToken(args[i+1:]) == "var"
	}
	return false
}

func nextCommandToken(args []string) string {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-e", "--env", "-t", "--template", "--env-file":
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}
