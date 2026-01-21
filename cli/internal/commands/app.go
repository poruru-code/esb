// Where: cli/internal/commands/app.go
// What: CLI entrypoint logic.
// Why: Provide a testable command dispatcher.
package commands

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// Dependencies holds all injected dependencies required for CLI command execution.
// This structure enables dependency injection for testing and allows swapping
// implementations of various subsystems.
type Dependencies struct {
	ProjectDir          string
	Out                 io.Writer
	DetectorFactory     helpers.DetectorFactory
	DockerClientFactory helpers.DockerClientFactory
	Now                 func() time.Time
	Prompter            interaction.Prompter
	RepoResolver        func(string) (string, error)
	GlobalConfigLoader  helpers.GlobalConfigLoader
	ProjectConfigLoader helpers.ProjectConfigLoader
	ProjectDirFinder    helpers.ProjectDirFinder
	Build               BuildDeps
}

// CLI defines the command-line interface structure parsed by Kong.
// It contains global flags and all subcommand definitions.
type CLI struct {
	Template   string        `short:"t" help:"Path to SAM template"`
	EnvFlag    string        `short:"e" name:"env" help:"Environment (default: last used)"`
	EnvFile    string        `name:"env-file" help:"Path to .env file"`
	Build      BuildCmd      `cmd:"" help:"Build images"`
	Env        EnvCmd        `cmd:"" name:"env" help:"Manage environments"`
	Project    ProjectCmd    `cmd:"" help:"Manage projects"`
	Config     ConfigCmd     `cmd:"" name:"config" help:"Manage configuration"`
	Complete   CompleteCmd   `cmd:"" name:"__complete" hidden:"" help:"Completion candidate provider"`
	Completion CompletionCmd `cmd:"" help:"Generate shell completion script"`
	Version    VersionCmd    `cmd:"" help:"Show version information"`
}

type (
	BuildCmd struct {
		NoCache bool `name:"no-cache" help:"Do not use cache when building images"`
		Verbose bool `short:"v" help:"Enable verbose output"`
		Force   bool `help:"Auto-unset invalid project/environment variables"`
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

type prefixHandler struct {
	prefix  string
	handler commandHandler
}

func dispatchCommand(command string, cli CLI, deps Dependencies, out io.Writer) (int, bool) {
	exactHandlers := map[string]commandHandler{
		"build":              runBuild,
		"env":                runEnvList,
		"env list":           runEnvList,
		"project":            runProjectList,
		"project list":       runProjectList,
		"project ls":         runProjectList,
		"project recent":     runProjectRecent,
		"config set-repo":    runConfigSetRepo,
		"__complete env":     runCompleteEnv,
		"__complete project": runCompleteProject,
		"completion bash":    func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionBash(cli, out) },
		"completion zsh":     func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionZsh(cli, out) },
		"completion fish":    func(_ CLI, _ Dependencies, out io.Writer) int { return runCompletionFish(cli, out) },
		"version":            func(_ CLI, _ Dependencies, out io.Writer) int { return runVersion(cli, out) },
	}

	if handler, ok := exactHandlers[command]; ok {
		return handler(cli, deps, out), true
	}

	prefixHandlers := []prefixHandler{
		{prefix: "env add", handler: runEnvAdd},
		{prefix: "env use", handler: runEnvUse},
		{prefix: "env remove", handler: runEnvRemove},
		{prefix: "project add", handler: runProjectAdd},
		{prefix: "project use", handler: runProjectUse},
		{prefix: "project remove", handler: runProjectRemove},
		{prefix: "config set-repo", handler: runConfigSetRepo},
	}

	for _, entry := range prefixHandlers {
		if strings.HasPrefix(command, entry.prefix) {
			return entry.handler(cli, deps, out), true
		}
	}

	return 1, false
}

// runVersion prints the version information of the CLI.
func runVersion(_ CLI, out io.Writer) int {
	legacyUI(out).Info(version.GetVersion())
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
			case "-e", "--env", "-t", "--template", "--env-file":
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
			selection, _ := resolveProjectSelection(CLI{}, deps, opts)
			if selection.Dir != "" {
				project, loadErr := loadProjectConfig(deps, selection.Dir)
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
			cfg, _ := loadGlobalConfigOrDefault(deps)
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
