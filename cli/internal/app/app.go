// Where: cli/internal/app/app.go
// What: CLI entrypoint logic.
// Why: Provide a testable command dispatcher.
package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// StateDetector defines the interface for detecting the current environment state.
// Implementations query Docker and file system to determine if the environment
// is running, stopped, or not initialized.
type StateDetector interface {
	Detect() (state.State, error)
}

// DetectorFactory is a function type that creates a StateDetector for a given
// project directory and environment name.
type DetectorFactory func(projectDir, env string) (StateDetector, error)

// Dependencies holds all injected dependencies required for CLI command execution.
// This structure enables dependency injection for testing and allows swapping
// implementations of various subsystems.
type Dependencies struct {
	ProjectDir      string
	Out             io.Writer
	DetectorFactory DetectorFactory
	Builder         Builder
	Downer          Downer
	Upper           Upper
	Stopper         Stopper
	Logger          Logger
	PortDiscoverer  PortDiscoverer
	Waiter          GatewayWaiter
	Provisioner     Provisioner
	Pruner          Pruner
	Now             func() time.Time
	Prompter        Prompter
	RepoResolver    func(string) (string, error)
}

// CLI defines the command-line interface structure parsed by Kong.
// It contains global flags and all subcommand definitions.
type CLI struct {
	Template   string        `short:"t" help:"Path to SAM template"`
	EnvFlag    string        `short:"e" name:"env" help:"Environment (default: last used)"`
	EnvFile    string        `name:"env-file" help:"Path to .env file"`
	Build      BuildCmd      `cmd:"" help:"Build images"`
	Up         UpCmd         `cmd:"" help:"Start environment"`
	Down       DownCmd       `cmd:"" help:"Stop environment"`
	Stop       StopCmd       `cmd:"" help:"Stop environment (preserve state)"`
	Logs       LogsCmd       `cmd:"" help:"View logs"`
	Prune      PruneCmd      `cmd:"" help:"Remove resources"`
	Env        EnvCmd        `cmd:"" name:"env" help:"Manage environments"`
	Project    ProjectCmd    `cmd:"" help:"Manage projects"`
	Config     ConfigCmd     `cmd:"" name:"config" help:"Manage configuration"`
	Complete   CompleteCmd   `cmd:"" name:"__complete" hidden:"" help:"Completion candidate provider"`
	Completion CompletionCmd `cmd:"" help:"Generate shell completion script"`
	Version    VersionCmd    `cmd:"" help:"Show version information"`
}

type VersionCmd struct{}

type (
	StopCmd struct {
		Force bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
	LogsCmd struct {
		Service    string `arg:"" optional:"" help:"Service name (default: all)"`
		Follow     bool   `short:"f" help:"Follow logs"`
		Tail       int    `help:"Tail the latest N lines"`
		Timestamps bool   `help:"Show timestamps"`
		Force      bool   `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
	}
)

type BuildCmd struct {
	NoCache bool `name:"no-cache" help:"Do not use cache when building images"`
	Verbose bool `short:"v" help:"Enable verbose output"`
	Force   bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
}
type UpCmd struct {
	Build  bool `help:"Rebuild before starting"`
	Reset  bool `help:"Reset environment before starting (down --volumes + build)"`
	Yes    bool `short:"y" help:"Skip confirmation prompt for --reset"`
	Detach bool `short:"d" default:"true" help:"Run in background"`
	Wait   bool `short:"w" help:"Wait for gateway ready"`
	Force  bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
}
type DownCmd struct {
	Volumes bool `short:"v" help:"Remove named volumes"`
	Force   bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
}
type PruneCmd struct {
	Yes     bool `short:"y" help:"Skip confirmation prompt"`
	All     bool `short:"a" help:"Remove all unused images (not just dangling)"`
	Volumes bool `help:"Remove unused volumes"`
	Hard    bool `help:"Also remove generator.yml"`
	Force   bool `help:"Auto-unset invalid ESB_PROJECT/ESB_ENV"`
}

// Run is the main entry point for CLI command execution.
// It parses the command-line arguments, identifies the requested command,
// and dispatches to the appropriate handler. Returns 0 on success, 1 on error.
func Run(args []string, deps Dependencies) int {
	out := deps.Out
	if out == nil {
		out = os.Stdout
	}

	if commandName(args) == "node" {
		fmt.Fprintln(out, "node command is disabled in Go CLI")
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
		return runNoArgs(deps, out)
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
			fmt.Fprintf(out, "Warning: failed to load env file %s: %v\n", cli.EnvFile, err)
		}
	} else {
		// Default to .env in current directory
		if _, err := os.Stat(".env"); err == nil {
			if err := godotenv.Load(); err != nil {
				fmt.Fprintf(out, "Warning: failed to load .env: %v\n", err)
			}
		}
	}

	command := ctx.Command()
	switch {
	case command == "build":
		return runBuild(cli, deps, out)
	case command == "up":
		return runUp(cli, deps, out)
	case command == "down":
		return runDown(cli, deps, out)
	case command == "stop":
		return runStop(cli, deps, out)
	case command == "prune":
		return runPrune(cli, deps, out)
	case strings.HasPrefix(command, "logs"):
		return runLogs(cli, deps, out)
	case command == "env" || command == "env list":
		return runEnvList(cli, deps, out)
	case strings.HasPrefix(command, "env add"):
		return runEnvAdd(cli, deps, out)
	case strings.HasPrefix(command, "env use"):
		return runEnvUse(cli, deps, out)
	case strings.HasPrefix(command, "env remove"):
		return runEnvRemove(cli, deps, out)
	case strings.HasPrefix(command, "env var"):
		return runEnvVar(cli, deps, out)
	case strings.HasPrefix(command, "project add"):
		return runProjectAdd(cli, deps, out)
	case command == "project recent":
		return runProjectRecent(cli, deps, out)
	case strings.HasPrefix(command, "project use"):
		return runProjectUse(cli, deps, out)
	case strings.HasPrefix(command, "project remove"):
		return runProjectRemove(cli, deps, out)
	case command == "project" || command == "project list" || command == "project ls":
		return runProjectList(cli, deps, out)
	case strings.HasPrefix(command, "config set-repo"):
		return runConfigSetRepo(cli, deps, out)
	case command == "__complete env":
		return runCompleteEnv(cli, deps, out)
	case command == "__complete project":
		return runCompleteProject(cli, deps, out)
	case command == "__complete service":
		return runCompleteService(cli, deps, out)
	case command == "completion bash":
		return runCompletionBash(cli, out)
	case command == "completion zsh":
		return runCompletionZsh(cli, out)
	case command == "completion fish":
		return runCompletionFish(cli, out)
	case command == "version":
		return runVersion(cli, out)
	default:
		fmt.Fprintln(out, "unknown command")
		return 1
	}
}

// runVersion prints the version information of the CLI.
func runVersion(_ CLI, out io.Writer) int {
	fmt.Fprintln(out, version.GetVersion())
	return 0
}

// runInitCommand executes the 'init' command which initializes a new project
// splitEnvList splits a comma-separated string of environment names
// into a slice. Returns nil if the input is empty.
func splitEnvList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return parts
}

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
			case "-e", "--env", "-t", "--template":
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}

// runNoArgs handles the case when esb is invoked without arguments.
// It displays full configuration and state information (equivalent to the old 'info' command).
func runNoArgs(deps Dependencies, out io.Writer) int {
	return runInfo(CLI{}, deps, out)
}

// handleParseError provides user-friendly error messages for parse failures.
func handleParseError(args []string, err error, deps Dependencies, out io.Writer) int {
	errStr := err.Error()

	// Handle missing required argument
	if strings.Contains(errStr, "expected") {
		cmd := commandName(args)
		switch {
		case strings.HasPrefix(cmd, "env") && strings.Contains(errStr, "<name>"):
			opts := newResolveOptions(false)
			selection, _ := resolveProjectSelection(CLI{}, Dependencies{}, opts)
			if selection.Dir != "" {
				project, loadErr := loadProjectConfig(selection.Dir)
				if loadErr == nil {
					var envNames []string
					for _, env := range project.Generator.Environments {
						envNames = append(envNames, env.Name)
					}
					return exitWithSuggestionAndAvailable(out,
						"Environment name required.",
						[]string{"esb env use <name>", "esb env list"},
						envNames,
					)
				}
			}
			return exitWithSuggestion(out, "Environment name required.",
				[]string{"esb env use <name>", "esb env list"})

		case cmd == "env" && strings.Contains(errStr, "expected one of"):
			return runEnvList(CLI{}, deps, out)

		case strings.HasPrefix(cmd, "project") && strings.Contains(errStr, "<name>"):
			cfg, _ := loadGlobalConfigOrDefault()
			var projectNames []string
			for name := range cfg.Projects {
				projectNames = append(projectNames, name)
			}
			return exitWithSuggestionAndAvailable(out,
				"Project name required.",
				[]string{"esb project use <name>", "esb project list"},
				projectNames,
			)

		case cmd == "project" && strings.Contains(errStr, "expected one of"):
			return runProjectList(CLI{}, deps, out)
		}
	}

	return exitWithError(out, err)
}
