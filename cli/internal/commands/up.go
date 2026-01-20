// Where: cli/internal/commands/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

// runUp executes the 'up' command which starts all services,
// optionally rebuilds images, provisions Lambda functions, and waits for readiness.
func runUp(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Up.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	if cli.Up.Reset && !cli.Up.Yes {
		if !interaction.IsTerminal(os.Stdin) {
			return exitWithError(out, fmt.Errorf("up --reset requires --yes in non-interactive mode"))
		}
		confirmed, err := interaction.PromptYesNo("Are you sure you want to continue?")
		if err != nil {
			return exitWithError(out, err)
		}
		if !confirmed {
			legacyUI(out).Info("Aborted.")
			return 1
		}
	}

	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	cmd, err := newUpCommand(deps.Up, repoResolver, out)
	if err != nil {
		return exitWithError(out, err)
	}

	if err := cmd.Run(ctxInfo, cli.Up, cli.EnvFile); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type upCommand struct {
	builder           ports.Builder
	upper             ports.Upper
	downer            ports.Downer
	publisher         ports.PortPublisher
	credentialManager ports.CredentialManager
	templateLoader    ports.TemplateLoader
	templateParser    ports.TemplateParser
	provisioner       ports.Provisioner
	waiter            ports.GatewayWaiter
	envApplier        ports.RuntimeEnvApplier
	ui                ports.UserInterface
}

func newUpCommand(deps UpDeps, repoResolver func(string) (string, error), out io.Writer) (*upCommand, error) {
	if deps.Upper == nil {
		return nil, fmt.Errorf("up: upper not configured")
	}
	if deps.Provisioner == nil {
		return nil, fmt.Errorf("up: provisioner not configured")
	}
	if deps.Parser == nil {
		return nil, fmt.Errorf("up: parser not configured")
	}
	envApplier := helpers.NewRuntimeEnvApplier(repoResolver)
	ui := ports.NewLegacyUI(out)
	return &upCommand{
		builder:           deps.Builder,
		upper:             deps.Upper,
		downer:            deps.Downer,
		publisher:         helpers.NewPortPublisher(deps.PortDiscoverer),
		credentialManager: helpers.NewCredentialManager(),
		templateLoader:    helpers.NewTemplateLoader(),
		templateParser:    helpers.NewTemplateParser(deps.Parser),
		provisioner:       deps.Provisioner,
		waiter:            deps.Waiter,
		envApplier:        envApplier,
		ui:                ui,
	}, nil
}

func (c *upCommand) Run(ctxInfo commandContext, flags UpCmd, envFile string) error {
	if c.ui == nil {
		return errors.New("up: ui not configured")
	}

	if c.envApplier != nil {
		c.envApplier.Apply(ctxInfo.Context)
	}

	if flags.Reset {
		printResetWarning(c.ui)
		if c.downer == nil {
			return errors.New("downer not configured")
		}
		if err := c.downer.Down(ctxInfo.Context.ComposeProject, true); err != nil {
			return err
		}
	}

	if c.credentialManager != nil {
		creds := c.credentialManager.Ensure()
		if creds.Generated {
			printCredentials(c.ui, creds)
		}
	}

	if flags.Build || flags.Reset {
		if c.builder == nil {
			return errors.New("builder not configured")
		}
		if err := c.builder.Build(manifest.BuildRequest{
			ProjectDir:   ctxInfo.Context.ProjectDir,
			ProjectName:  ctxInfo.Context.ComposeProject,
			TemplatePath: resolvedTemplatePath(ctxInfo),
			Env:          ctxInfo.Env,
		}); err != nil {
			return err
		}
	}

	if c.upper == nil {
		return errors.New("upper not configured")
	}
	if err := c.upper.Up(ports.UpRequest{
		Context: ctxInfo.Context,
		Detach:  flags.Detach,
		Wait:    flags.Wait,
		EnvFile: envFile,
	}); err != nil {
		return err
	}

	var discoveredPorts map[string]int
	if c.publisher != nil {
		portsDiscovered, err := c.publisher.Publish(ctxInfo.Context)
		if err != nil {
			c.ui.Warn(err.Error())
		} else {
			discoveredPorts = portsDiscovered
		}
	}

	if c.templateLoader == nil || c.templateParser == nil || c.provisioner == nil {
		return errors.New("template pipeline not configured")
	}
	content, err := c.templateLoader.Read(resolvedTemplatePath(ctxInfo))
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}
	parsed, err := c.templateParser.Parse(content, nil)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	if err := c.provisioner.Apply(context.Background(), parsed.Resources, ctxInfo.Context.ComposeProject); err != nil {
		return err
	}

	if flags.Wait {
		if c.waiter == nil {
			return errors.New("waiter not configured")
		}
		if err := c.waiter.Wait(ctxInfo.Context); err != nil {
			return err
		}
	}

	c.ui.Success("✓ Up complete")
	c.ui.Info("Next:")
	c.ui.Info("  esb logs <service>  # View logs")
	c.ui.Info("  esb down            # Stop environment")
	printDiscoveredPorts(c.ui, discoveredPorts)
	return nil
}

func printCredentials(ui ports.UserInterface, creds ports.AuthCredentials) {
	if ui == nil {
		return
	}
	rows := []ports.KeyValue{
		{Key: "AUTH_USER", Value: creds.AuthUser},
		{Key: "AUTH_PASS", Value: creds.AuthPass},
		{Key: "JWT_SECRET_KEY", Value: creds.JWTSecretKey},
		{Key: "X_API_KEY", Value: creds.XAPIKey},
		{Key: "RUSTFS_ACCESS_KEY", Value: creds.RustfsAccessKey},
		{Key: "RUSTFS_SECRET_KEY", Value: creds.RustfsSecretKey},
	}
	ui.Block("🔑", "Authentication credentials:", rows)
}

func printDiscoveredPorts(ui ports.UserInterface, portsMap map[string]int) {
	if ui == nil || len(portsMap) == 0 {
		return
	}

	var rows []ports.KeyValue
	add := func(key string) {
		if val, ok := portsMap[key]; ok {
			rows = append(rows, ports.KeyValue{Key: key, Value: val})
		}
	}

	add(constants.EnvPortGatewayHTTPS)
	add(constants.EnvPortVictoriaLogs)
	add(constants.EnvPortDatabase)
	add(constants.EnvPortS3)
	add(constants.EnvPortS3Mgmt)
	add(constants.EnvPortRegistry)

	var unknown []string
	for key := range portsMap {
		if !containsKnown(key) {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		rows = append(rows, ports.KeyValue{Key: key, Value: portsMap[key]})
	}

	if len(rows) == 0 {
		return
	}
	ui.Block("🔌", "Discovered Ports:", rows)
}

func printResetWarning(ui ports.UserInterface) {
	if ui == nil {
		return
	}
	ui.Warn("WARNING! This will remove:")
	ui.Warn("  - all containers for the selected environment")
	ui.Warn("  - all volumes for the selected environment (DB/S3 data)")
	ui.Warn("  - rebuild images and restart services")
	ui.Warn("")
}

func containsKnown(key string) bool {
	switch key {
	case constants.EnvPortGatewayHTTPS, constants.EnvPortVictoriaLogs, constants.EnvPortDatabase, constants.EnvPortS3, constants.EnvPortS3Mgmt, constants.EnvPortRegistry, constants.EnvPortAgentGRPC:
		return true
	}
	return false
}
