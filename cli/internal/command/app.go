// Where: cli/internal/command/app.go
// What: CLI entrypoint logic.
// Why: Provide a testable command dispatcher.
package command

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	usecasedeploy "github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

const (
	repoRequiredExitCode     = 2
	repoRequiredErrorMessage = "EBS repository root not found from current directory. Run this command inside the EBS repository."
	cliCommandName           = "esb"
)

var commandValueFlags = collectValueFlags(reflect.TypeOf(CLI{}))

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
	Template []string    `short:"t" help:"Path to SAM template (repeatable)"`
	EnvFlag  string      `short:"e" name:"env" help:"Environment name"`
	EnvFile  string      `name:"env-file" help:"Path to .env file"`
	Deploy   DeployCmd   `cmd:"" help:"Deploy functions"`
	Artifact ArtifactCmd `cmd:"" help:"Artifact operations"`
	Version  VersionCmd  `cmd:"" help:"Show version information"`
}

type (
	// DeployCmd defines the deploy command flags.
	DeployCmd struct {
		Mode         string   `short:"m" help:"Runtime mode (docker/containerd)"`
		Output       string   `short:"o" help:"Output directory for generated artifacts"`
		Manifest     string   `name:"manifest" help:"Output path for artifact manifest (artifact.yml)"`
		Project      string   `short:"p" help:"Compose project name to target"`
		ComposeFiles []string `name:"compose-file" sep:"," help:"Compose file(s) to use (repeatable or comma-separated)"`
		ImageURI     []string `name:"image-uri" sep:"," help:"Image URI override for image functions (<function>=<image-uri>)"`
		ImageRuntime []string `name:"image-runtime" sep:"," help:"Runtime override for image functions (<function>=<python|java21>)"`
		BuildOnly    bool     `name:"build-only" help:"Build only (skip provisioner and runtime sync)"`
		Bundle       bool     `name:"bundle-manifest" help:"Write bundle manifest (for bundling)"`
		NoCache      bool     `name:"no-cache" help:"Do not use cache when building images"`
		WithDeps     bool     `name:"with-deps" help:"Start dependent services when running provisioner"`
		SecretEnv    string   `name:"secret-env" help:"Path to secret env file for apply phase"`
		Strict       bool     `name:"strict" help:"Enable strict runtime metadata validation for apply phase"`
		Verbose      bool     `short:"v" help:"Verbose output"`
		Emoji        bool     `name:"emoji" help:"Enable emoji output (default: auto)"`
		NoEmoji      bool     `name:"no-emoji" help:"Disable emoji output"`
		Force        bool     `help:"Allow environment mismatch with running gateway (skip auto-alignment)"`
		NoSave       bool     `name:"no-save-defaults" help:"Do not persist deploy defaults"`
	}

	ArtifactCmd struct {
		Generate ArtifactGenerateCmd `cmd:"" help:"Generate artifacts and manifest (without apply)"`
		Apply    ArtifactApplyCmd    `cmd:"" help:"Apply artifact manifest"`
	}

	ArtifactGenerateCmd struct {
		Mode         string   `short:"m" help:"Runtime mode (docker/containerd)"`
		Output       string   `short:"o" help:"Output directory for generated artifacts"`
		Manifest     string   `name:"manifest" help:"Output path for artifact manifest (artifact.yml)"`
		Project      string   `short:"p" help:"Compose project name to target"`
		ComposeFiles []string `name:"compose-file" sep:"," help:"Compose file(s) to use (repeatable or comma-separated)"`
		ImageURI     []string `name:"image-uri" sep:"," help:"Image URI override for image functions (<function>=<image-uri>)"`
		ImageRuntime []string `name:"image-runtime" sep:"," help:"Runtime override for image functions (<function>=<python|java21>)"`
		Bundle       bool     `name:"bundle-manifest" help:"Write bundle manifest (for bundling)"`
		BuildImages  bool     `name:"build-images" help:"Build base/function images during generate"`
		NoCache      bool     `name:"no-cache" help:"Do not use cache when building images"`
		Verbose      bool     `short:"v" help:"Verbose output"`
		Emoji        bool     `name:"emoji" help:"Enable emoji output (default: auto)"`
		NoEmoji      bool     `name:"no-emoji" help:"Disable emoji output"`
		Force        bool     `help:"Allow environment mismatch with running gateway (skip auto-alignment)"`
		NoSave       bool     `name:"no-save-defaults" help:"Do not persist deploy defaults"`
	}

	ArtifactApplyCmd struct {
		Artifact  string `name:"artifact" help:"Path to artifact manifest (artifact.yml)"`
		OutputDir string `name:"out" help:"Output config directory"`
		SecretEnv string `name:"secret-env" help:"Path to secret env file"`
		Strict    bool   `name:"strict" help:"Enable strict runtime metadata validation"`
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
		ComposeProvisioner        usecasedeploy.ComposeProvisioner
		ComposeProvisionerFactory func(ui.UserInterface) usecasedeploy.ComposeProvisioner
		NewDeployUI               func(io.Writer, bool) ui.UserInterface
	}
)

type (
	RegistryWaiter func(registry string, timeout time.Duration) error

	DockerClientFactory func() (compose.DockerClient, error)

	ComposeProvisioner = usecasedeploy.ComposeProvisioner
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

	if exitCode, blocked := enforceRepoScope(args, deps, out); blocked {
		return exitCode
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
		"deploy":            runDeploy,
		"artifact generate": runArtifactGenerate,
		"artifact apply":    runArtifactApply,
		"version":           func(_ CLI, _ Dependencies, out io.Writer) int { return runVersion(cli, out) },
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
			if commandFlagExpectsValue(arg) {
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}

func commandFlagExpectsValue(arg string) bool {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" || trimmed == "-" || trimmed == "--" {
		return false
	}
	if strings.Contains(trimmed, "=") {
		return false
	}
	_, ok := commandValueFlags[trimmed]
	return ok
}

func collectValueFlags(root reflect.Type) map[string]struct{} {
	out := map[string]struct{}{}
	collectValueFlagsRecursive(root, out)
	return out
}

func collectValueFlagsRecursive(current reflect.Type, out map[string]struct{}) {
	for current.Kind() == reflect.Pointer {
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < current.NumField(); i++ {
		field := current.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if _, isCommand := field.Tag.Lookup("cmd"); isCommand {
			collectValueFlagsRecursive(field.Type, out)
			continue
		}
		if !fieldRequiresValue(field.Type) {
			continue
		}
		if shortName := strings.TrimSpace(field.Tag.Get("short")); shortName != "" {
			out["-"+shortName] = struct{}{}
		}
		longName := strings.TrimSpace(field.Tag.Get("name"))
		if longName == "" {
			longName = toKebabCase(field.Name)
		}
		if longName != "" {
			out["--"+longName] = struct{}{}
		}
	}
}

func fieldRequiresValue(fieldType reflect.Type) bool {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch fieldType.Kind() {
	case reflect.Bool:
		return false
	case reflect.Struct:
		return false
	default:
		return true
	}
}

func toKebabCase(value string) string {
	var out strings.Builder
	for i, r := range value {
		if unicode.IsUpper(r) {
			if i > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(unicode.ToLower(r))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func enforceRepoScope(args []string, deps Dependencies, out io.Writer) (int, bool) {
	if !requiresRepoScope(args) {
		return 0, false
	}
	if _, err := deps.RepoResolver(""); err != nil {
		legacyUI(out).Warn(repoRequiredErrorMessage)
		return repoRequiredExitCode, true
	}
	return 0, false
}

func requiresRepoScope(args []string) bool {
	if commandName(args) == "version" {
		return false
	}
	return !isHelpCommand(args)
}

func isHelpCommand(args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-h", "--help":
				return true
			case "--":
				return false
			}
			if commandFlagExpectsValue(arg) {
				skipNext = true
			}
			continue
		}
	}
	return false
}

// runNoArgs handles the case when the CLI is invoked without arguments.
// It displays full configuration and state information (equivalent to the old 'info' command).
func runNoArgs(out io.Writer) int {
	ui := legacyUI(out)
	ui.Info("Usage:")
	ui.Info(fmt.Sprintf("  %s deploy --template <path> --env <name> --mode <docker|containerd> [flags]", cliCommandName))
	ui.Info("")
	ui.Info(fmt.Sprintf("Try: %s deploy --help", cliCommandName))
	return 0
}

// handleParseError provides user-friendly error messages for parse failures.
func handleParseError(args []string, err error, deps Dependencies, out io.Writer) int {
	_ = args
	_ = deps
	msg := err.Error()
	if strings.Contains(msg, "expected string value") {
		ui := legacyUI(out)
		switch {
		case strings.Contains(msg, "--template"):
			ui.Warn("`-t/--template` expects a value. Provide a path (repeatable) or omit for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -t ./template.yaml -t ./extra.yaml", cliCommandName))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cliCommandName))
			return 1
		case strings.Contains(msg, "--env"):
			ui.Warn("`-e/--env` expects a value. Provide a name or omit the flag for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -e prod", cliCommandName))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cliCommandName))
			return 1
		case strings.Contains(msg, "--mode"):
			ui.Warn("`-m/--mode` expects a value. Use docker/containerd or omit the flag for interactive input.")
			ui.Info(fmt.Sprintf("Example: %s deploy -m docker", cliCommandName))
			ui.Info(fmt.Sprintf("Interactive: %s deploy", cliCommandName))
			return 1
		case strings.Contains(msg, "--env-file"):
			ui.Warn("`--env-file` expects a value. Provide a file path.")
			ui.Info(fmt.Sprintf("Example: %s deploy --env-file .env.prod", cliCommandName))
			return 1
		case strings.Contains(msg, "--image-uri"):
			ui.Warn("`--image-uri` expects a value. Use <function>=<image-uri>.")
			ui.Info(fmt.Sprintf("Example: %s deploy --image-uri lambda-image=public.ecr.aws/example/repo:latest", cliCommandName))
			return 1
		case strings.Contains(msg, "--image-runtime"):
			ui.Warn("`--image-runtime` expects a value. Use <function>=<python|java21>.")
			ui.Info(fmt.Sprintf("Example: %s deploy --image-runtime lambda-image=java21", cliCommandName))
			return 1
		case strings.Contains(msg, "--manifest"):
			ui.Warn("`--manifest` expects a value. Provide an output artifact manifest path.")
			ui.Info(fmt.Sprintf("Example: %s artifact generate --manifest e2e/artifacts/e2e-docker/artifact.yml", cliCommandName))
			return 1
		}
	}
	return exitWithError(out, err)
}
