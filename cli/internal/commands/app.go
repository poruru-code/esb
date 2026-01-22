// Where: cli/internal/commands/app.go
// What: CLI entrypoint logic.
// Why: Provide a testable command dispatcher.
package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// Dependencies holds all injected dependencies required for CLI command execution.
// This structure enables dependency injection for testing and allows swapping
// implementations of various subsystems.
type Dependencies struct {
	Out          io.Writer
	Prompter     interaction.Prompter
	RepoResolver func(string) (string, error)
	Build        BuildDeps
}

// CLI defines the command-line interface structure parsed by Kong.
// It contains global flags and all subcommand definitions.
type CLI struct {
	Template   string        `short:"t" help:"Path to SAM template"`
	EnvFlag    string        `short:"e" name:"env" help:"Environment name"`
	EnvFile    string        `name:"env-file" help:"Path to .env file"`
	Build      BuildCmd      `cmd:"" help:"Build images"`
	Completion CompletionCmd `cmd:"" help:"Generate shell completion script"`
	Version    VersionCmd    `cmd:"" help:"Show version information"`
}

type (
	BuildCmd struct {
		Mode    string `short:"m" help:"Runtime mode (docker/containerd/firecracker)"`
		Output  string `short:"o" help:"Output directory for generated artifacts"`
		NoCache bool   `name:"no-cache" help:"Do not use cache when building images"`
		Verbose bool   `short:"v" help:"Enable verbose output"`
		Force   bool   `help:"Auto-unset invalid project/environment variables"`
	}
	VersionCmd struct{}

	BuildDeps struct {
		Builder Builder
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
	ui := legacyUI(out)

	if commandName(args) == "node" {
		ui.Warn("node command is disabled in Go CLI")
		return 1
	}

	if err := config.EnsureGlobalConfig(); err != nil {
		return exitWithError(out, err)
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
		"build":           runBuild,
		"completion bash": func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionBash(cli, out) },
		"completion zsh":  func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionZsh(cli, out) },
		"completion fish": func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionFish(cli, out) },
		"version":         func(_ CLI, _ Dependencies, out io.Writer) int { return runVersion(cli, out) },
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
			case "-e", "--env", "-t", "--template", "--env-file", "-m", "--mode", "-o", "--output":
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}

// CommandName exposes command parsing for wiring decisions.
func CommandName(args []string) string {
	return commandName(args)
}

// runNoArgs handles the case when esb is invoked without arguments.
// It displays full configuration and state information (equivalent to the old 'info' command).
func runNoArgs(out io.Writer) int {
	ui := legacyUI(out)
	ui.Info("Usage:")
	ui.Info("  esb build --template <path> --env <name> --mode <docker|containerd|firecracker> [flags]")
	ui.Info("")
	ui.Info("Try: esb build --help")
	return 0
}

// handleParseError provides user-friendly error messages for parse failures.
func handleParseError(args []string, err error, deps Dependencies, out io.Writer) int {
	_ = args
	_ = deps
	return exitWithError(out, err)
}
