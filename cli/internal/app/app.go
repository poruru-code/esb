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
	"github.com/poruru/edge-serverless-box/cli/internal/state"
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
}

// CLI defines the command-line interface structure parsed by Kong.
// It contains global flags and all subcommand definitions.
type CLI struct {
	Template string     `short:"t" help:"Path to SAM template"`
	EnvFlag  string     `short:"e" name:"env" help:"Environment (default: active)"`
	Init     InitCmd    `cmd:"" help:"Initialize project"`
	Build    BuildCmd   `cmd:"" help:"Build images"`
	Up       UpCmd      `cmd:"" help:"Start environment"`
	Down     DownCmd    `cmd:"" help:"Stop environment"`
	Stop     StopCmd    `cmd:"" help:"Stop environment (preserve state)"`
	Logs     LogsCmd    `cmd:"" help:"View logs"`
	Reset    ResetCmd   `cmd:"" help:"Reset environment"`
	Prune    PruneCmd   `cmd:"" help:"Remove resources"`
	Status   StatusCmd  `cmd:"" help:"Show state"`
	Info     InfoCmd    `cmd:"" help:"Show configuration and state"`
	Env      EnvCmd     `cmd:"" name:"env" help:"Manage environments"`
	Project  ProjectCmd `cmd:"" help:"Manage projects"`
}

type (
	StatusCmd struct{}
	InfoCmd   struct{}
	StopCmd   struct{}
	LogsCmd   struct {
		Service    string `arg:"" optional:"" help:"Service name (default: all)"`
		Follow     bool   `short:"f" help:"Follow logs"`
		Tail       int    `help:"Tail the latest N lines"`
		Timestamps bool   `help:"Show timestamps"`
	}
)

type InitCmd struct {
	Name string `short:"n" help:"Project name (default: directory)"`
}
type BuildCmd struct {
	NoCache bool `name:"no-cache" help:"Do not use cache when building images"`
}
type UpCmd struct {
	Build  bool `help:"Rebuild before starting"`
	Detach bool `short:"d" default:"true" help:"Run in background"`
	Wait   bool `short:"w" help:"Wait for gateway ready"`
}
type DownCmd struct {
	Volumes bool `short:"v" help:"Remove named volumes"`
}
type ResetCmd struct {
	Yes bool `short:"y" help:"Skip confirmation"`
}
type PruneCmd struct {
	Yes  bool `short:"y" help:"Skip confirmation"`
	Hard bool `help:"Also remove generator.yml"`
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

	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	ctx, err := parser.Parse(args)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	command := ctx.Command()
	switch {
	case command == "init":
		return runInitCommand(cli, deps, out)
	case command == "build":
		return runBuild(cli, deps, out)
	case command == "up":
		return runUp(cli, deps, out)
	case command == "down":
		return runDown(cli, deps, out)
	case command == "stop":
		return runStop(cli, deps, out)
	case command == "reset":
		return runReset(cli, deps, out)
	case command == "prune":
		return runPrune(cli, deps, out)
	case command == "status":
		return runStatus(cli, deps, out)
	case command == "info":
		return runInfo(cli, deps, out)
	case strings.HasPrefix(command, "logs"):
		return runLogs(cli, deps, out)
	case command == "env list":
		return runEnvList(cli, deps, out)
	case strings.HasPrefix(command, "env create"):
		return runEnvCreate(cli, deps, out)
	case strings.HasPrefix(command, "env use"):
		return runEnvUse(cli, deps, out)
	case strings.HasPrefix(command, "env remove"):
		return runEnvRemove(cli, deps, out)
	case command == "project list":
		return runProjectList(cli, deps, out)
	case command == "project recent":
		return runProjectRecent(cli, deps, out)
	case strings.HasPrefix(command, "project use"):
		return runProjectUse(cli, deps, out)
	default:
		fmt.Fprintln(out, "unknown command")
		return 1
	}
}

// runStatus executes the 'status' command which displays the current
// environment state (running, stopped, or not initialized).
func runStatus(cli CLI, deps Dependencies, out io.Writer) int {
	factory := deps.DetectorFactory
	if factory == nil {
		fmt.Fprintln(out, "detector factory not configured")
		return 1
	}

	selection, err := resolveProjectSelection(cli, deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	projectDir := selection.Dir
	if projectDir == "" {
		projectDir = "."
	}

	envDeps := deps
	envDeps.ProjectDir = projectDir
	env := resolveEnv(cli, envDeps)

	detector, err := factory(projectDir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	stateValue, err := detector.Detect()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, stateValue)
	return 0
}

// runInitCommand executes the 'init' command which initializes a new project
// by creating a generator.yml configuration file from the specified SAM template.
func runInitCommand(cli CLI, deps Dependencies, out io.Writer) int {
	if cli.Template == "" {
		fmt.Fprintln(out, "template is required")
		return 1
	}

	envs := splitEnvList(cli.EnvFlag)
	path, err := runInit(cli.Template, envs, cli.Init.Name)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if err := registerProject(path, deps); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Configuration saved to: %s\n", path)
	return 0
}

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
