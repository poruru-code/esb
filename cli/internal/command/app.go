// Where: cli/internal/command/app.go
// What: CLI entrypoint logic.
// Why: Provide a testable command dispatcher.
package command

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// Dependencies holds all injected dependencies required for CLI command execution.
// This structure enables dependency injection for testing and allows swapping
// implementations of various subsystems.
type Dependencies struct {
	Out          io.Writer
	ErrOut       io.Writer
	Prompter     interaction.Prompter
	RepoResolver func(string) (string, error)
	Deploy       DeployDeps
}

// CLI defines the command-line interface structure parsed by Kong.
// It contains global flags and all subcommand definitions.
type CLI struct {
	Template []string   `short:"t" help:"Path to SAM template (repeatable)"`
	EnvFlag  string     `short:"e" name:"env" help:"Environment name"`
	EnvFile  string     `name:"env-file" help:"Path to .env file"`
	Deploy   DeployCmd  `cmd:"" help:"Deploy functions"`
	Version  VersionCmd `cmd:"" help:"Show version information"`
}

type (
	// DeployCmd defines the deploy command flags.
	DeployCmd struct {
		Mode         string   `short:"m" help:"Runtime mode (docker/containerd)"`
		Output       string   `short:"o" help:"Output directory for generated artifacts"`
		Project      string   `short:"p" help:"Compose project name to target"`
		ComposeFiles []string `name:"compose-file" sep:"," help:"Compose file(s) to use (repeatable or comma-separated)"`
		BuildOnly    bool     `name:"build-only" help:"Build only (skip provisioner and runtime sync)"`
		Bundle       bool     `name:"bundle-manifest" help:"Write bundle manifest (for bundling)"`
		ImagePrewarm string   `name:"image-prewarm" default:"all" help:"Image prewarm mode (all/off)"`
		NoCache      bool     `name:"no-cache" help:"Do not use cache when building images"`
		NoDeps       bool     `name:"no-deps" help:"Do not start dependent services when running provisioner (default)"`
		WithDeps     bool     `name:"with-deps" help:"Start dependent services when running provisioner"`
		Verbose      bool     `short:"v" help:"Verbose output"`
		Emoji        bool     `name:"emoji" help:"Enable emoji output (default: auto)"`
		NoEmoji      bool     `name:"no-emoji" help:"Disable emoji output"`
		Force        bool     `help:"Allow environment mismatch with running gateway (skip auto-alignment)"`
		NoSave       bool     `name:"no-save-defaults" help:"Do not persist deploy defaults"`
	}

	VersionCmd struct{}

	DeployDeps struct {
		Build     DeployBuildDeps
		Runtime   DeployRuntimeDeps
		Provision DeployProvisionDeps
	}

	DeployBuildDeps struct {
		Build func(build.BuildRequest) error
	}

	DeployRuntimeDeps struct {
		ApplyRuntimeEnv    func(state.Context) error
		RuntimeEnvResolver runtimeinfra.EnvResolver
		RegistryWaiter     RegistryWaiter
		DockerClient       DockerClientFactory
	}

	DeployProvisionDeps struct {
		ComposeRunner             compose.CommandRunner
		ComposeProvisioner        ComposeProvisioner
		ComposeProvisionerFactory func(ui.UserInterface) ComposeProvisioner
		NewDeployUI               func(io.Writer, bool) ui.UserInterface
	}
)

type (
	RegistryWaiter func(registry string, timeout time.Duration) error

	DockerClientFactory func() (compose.DockerClient, error)

	ComposeProvisioner interface {
		CheckServicesStatus(composeProject, mode string)
		RunProvisioner(
			composeProject string,
			mode string,
			noDeps bool,
			verbose bool,
			projectDir string,
			composeFiles []string,
		) error
	}
)

// Run is the main entry point for CLI command execution.
// It parses the command-line arguments, identifies the requested command,
// and dispatches to the appropriate handler. Returns 0 on success, 1 on error.
func Run(args []string, deps Dependencies) int {
	out := deps.Out
	if out == nil {
		out = os.Stdout
	}
	if deps.ErrOut == nil {
		deps.ErrOut = os.Stderr
	}
	ui := legacyUI(out)

	if commandName(args) == "node" {
		ui.Warn("node command is disabled in Go CLI")
		return 1
	}

	if deps.RepoResolver == nil {
		deps.RepoResolver = config.ResolveRepoRoot
	}

	// Handle no arguments: show current location and help
	if len(args) == 0 {
		return runNoArgs(out)
	}

	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		return exitWithError(out, err)
	}

	ctx, err := parser.Parse(args)
	if err != nil {
		return handleParseError(args, err, deps, out)
	}

	// Load environment file if provided or if .env exists in current directory
	if cli.EnvFile != "" {
		if err := godotenv.Load(cli.EnvFile); err != nil {
			ui.Warn(fmt.Sprintf("Warning: failed to load env file %s: %v", cli.EnvFile, err))
		}
	} else {
		// Default to .env in current directory
		if _, err := os.Stat(".env"); err == nil {
			if err := godotenv.Load(); err != nil {
				ui.Warn(fmt.Sprintf("Warning: failed to load .env: %v", err))
			}
		}
	}

	command := ctx.Command()
	if exitCode, handled := dispatchCommand(command, cli, deps, out); handled {
		return exitCode
	}

	ui.Warn("unknown command")
	return 1
}

type commandHandler func(CLI, Dependencies, io.Writer) int

func dispatchCommand(command string, cli CLI, deps Dependencies, out io.Writer) (int, bool) {
	exactHandlers := map[string]commandHandler{
		"deploy":  runDeploy,
		"version": func(_ CLI, _ Dependencies, out io.Writer) int { return runVersion(cli, out) },
	}

	if handler, ok := exactHandlers[command]; ok {
		return handler(cli, deps, out), true
	}

	return 1, false
}

// runVersion prints the version information of the CLI.
func runVersion(_ CLI, out io.Writer) int {
	legacyUI(out).Info(version.GetVersion())
	return 0
}

// runInitCommand executes the 'init' command which initializes a new project
// commandName extracts the first non-flag argument from the command line,
// which represents the command name. Recognizes and skips known flag pairs.
func commandName(args []string) string {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-e", "--env", "-t", "--template", "--env-file", "-m", "--mode", "-o", "--output", "-p", "--project", "--image-prewarm":
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}

// runNoArgs handles the case when the CLI is invoked without arguments.
// It displays full configuration and state information (equivalent to the old 'info' command).
func runNoArgs(out io.Writer) int {
	ui := legacyUI(out)
	cmd := cliName()
	ui.Info("Usage:")
	ui.Info(fmt.Sprintf("  %s deploy --template <path> --env <name> --mode <docker|containerd> [flags]", cmd))
	ui.Info("")
	ui.Info(fmt.Sprintf("Try: %s deploy --help", cmd))
	return 0
}

// handleParseError provides user-friendly error messages for parse failures.
func handleParseError(args []string, err error, deps Dependencies, out io.Writer) int {
	_ = args
	_ = deps
	msg := err.Error()
	if strings.Contains(msg, "expected string value") {
		ui := legacyUI(out)
		cmd := cliName()
		switch {
		case strings.Contains(msg, "--template"):
			ui.Warn("`-t/--template` expects a value. Provide a path (repeatable) or omit for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -t ./template.yaml -t ./extra.yaml", cmd))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cmd))
			return 1
		case strings.Contains(msg, "--env"):
			ui.Warn("`-e/--env` expects a value. Provide a name or omit the flag for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -e prod", cmd))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cmd))
			return 1
		case strings.Contains(msg, "--mode"):
			ui.Warn("`-m/--mode` expects a value. Use docker/containerd or omit the flag for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -m docker", cmd))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cmd))
			return 1
		case strings.Contains(msg, "--env-file"):
			ui.Warn("`--env-file` expects a value. Provide a file path.")
			ui.Info(fmt.Sprintf("Example: %s deploy --env-file .env.prod", cmd))
			return 1
		}
	}
	return exitWithError(out, err)
}
