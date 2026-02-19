package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore/composeprovision"
)

type CLI struct {
	Deploy    DeployCmd    `cmd:"" help:"Prepare images and apply artifact manifest"`
	Provision ProvisionCmd `cmd:"" help:"Run deploy provisioner via docker compose"`
}

type DeployCmd struct {
	Artifact  string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	Output    string `name:"out" required:"" help:"Output config directory (CONFIG_DIR)"`
	SecretEnv string `name:"secret-env" help:"Path to secret env file"`
	Strict    bool   `name:"strict" help:"Enable strict runtime metadata validation"`
	NoCache   bool   `name:"no-cache" help:"Do not use cache when building images"`
}

type ProvisionCmd struct {
	ComposeProject string   `name:"project" required:"" help:"Compose project name"`
	ComposeFiles   []string `name:"compose-file" required:"" sep:"," help:"Compose file(s) to use (repeatable or comma-separated)"`
	EnvFile        string   `name:"env-file" help:"Path to compose env file"`
	ProjectDir     string   `name:"project-dir" help:"Working directory for docker compose (default: current directory)"`
	WithDeps       bool     `name:"with-deps" help:"Start dependent services when running provisioner"`
	Verbose        bool     `short:"v" help:"Verbose output"`
}

type kongExitCode int

type commandDeps struct {
	executeDeploy    func(artifactcore.DeployInput) (artifactcore.ApplyResult, error)
	executeProvision func(ProvisionInput) error
	warningWriter    io.Writer
	out              io.Writer
	errOut           io.Writer
}

type ProvisionInput struct {
	ComposeProject string
	ComposeFiles   []string
	EnvFile        string
	ProjectDir     string
	NoDeps         bool
	Verbose        bool
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps()))
}

func defaultDeps() commandDeps {
	return commandDeps{
		executeDeploy:    artifactcore.ExecuteDeploy,
		executeProvision: executeProvision,
		warningWriter:    os.Stderr,
		out:              os.Stdout,
		errOut:           os.Stderr,
	}
}

func run(args []string, deps commandDeps) (exitCode int) {
	out := deps.out
	if out == nil {
		out = os.Stdout
	}
	errOut := deps.errOut
	if errOut == nil {
		errOut = os.Stderr
	}
	cli := CLI{}
	parser, err := kong.New(
		&cli,
		kong.Name("artifactctl"),
		kong.Description("Prepare images and apply generated artifact manifests."),
		kong.Writers(out, errOut),
		kong.Exit(func(code int) {
			panic(kongExitCode(code))
		}),
	)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: initialize command parser: %v\n", err)
		return 1
	}
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		code, ok := recovered.(kongExitCode)
		if !ok {
			panic(recovered)
		}
		exitCode = int(code)
	}()
	ctx, err := parser.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl --help`, `artifactctl deploy --help`, or `artifactctl provision --help`.")
		return 1
	}
	switch ctx.Command() {
	case "deploy":
		if err := runDeploy(cli.Deploy, deps, errOut); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			if hint := hintForDeployError(err); hint != "" {
				_, _ = fmt.Fprintf(errOut, "Hint: %s\n", hint)
			}
			return 1
		}
		return 0
	case "provision":
		if err := runProvision(cli.Provision, deps); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl provision --help` for required arguments.")
			return 1
		}
		return 0
	default:
		_, _ = fmt.Fprintf(errOut, "Error: unsupported command: %s\n", ctx.Command())
		_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl --help`.")
		return 1
	}
}

func runDeploy(cmd DeployCmd, deps commandDeps, errOut io.Writer) error {
	executeDeploy := deps.executeDeploy
	if executeDeploy == nil {
		executeDeploy = artifactcore.ExecuteDeploy
	}
	warningWriter := deps.warningWriter
	if warningWriter == nil {
		if errOut == nil {
			warningWriter = os.Stderr
		} else {
			warningWriter = errOut
		}
	}
	result, err := executeDeploy(artifactcore.DeployInput{
		Apply: artifactcore.ApplyInput{
			ArtifactPath:  cmd.Artifact,
			OutputDir:     cmd.Output,
			SecretEnvPath: cmd.SecretEnv,
			Strict:        cmd.Strict,
		},
		NoCache: cmd.NoCache,
	})
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintf(warningWriter, "Warning: %s\n", warning)
	}
	return nil
}

func runProvision(cmd ProvisionCmd, deps commandDeps) error {
	execProvision := deps.executeProvision
	if execProvision == nil {
		execProvision = executeProvision
	}
	return execProvision(ProvisionInput{
		ComposeProject: cmd.ComposeProject,
		ComposeFiles:   append([]string(nil), cmd.ComposeFiles...),
		EnvFile:        cmd.EnvFile,
		ProjectDir:     cmd.ProjectDir,
		NoDeps:         !cmd.WithDeps,
		Verbose:        cmd.Verbose,
	})
}

func executeProvision(input ProvisionInput) error {
	workingDir := strings.TrimSpace(input.ProjectDir)
	if workingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		workingDir = cwd
	}
	return composeprovision.Execute(
		context.Background(),
		osComposeRunner{},
		workingDir,
		composeprovision.Request{
			ComposeProject: input.ComposeProject,
			ComposeFiles:   input.ComposeFiles,
			EnvFile:        input.EnvFile,
			NoDeps:         input.NoDeps,
			Verbose:        input.Verbose,
		},
	)
}

type osComposeRunner struct{}

func (osComposeRunner) Run(ctx context.Context, cwd, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (osComposeRunner) RunQuiet(ctx context.Context, cwd, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func hintForDeployError(err error) string {
	var missingSecretKeys artifactcore.MissingSecretKeysError
	var missingReferencedPath artifactcore.MissingReferencedPathError

	switch {
	case errors.Is(err, artifactcore.ErrRuntimeBaseDockerfileMissing):
		return "run `esb artifact generate ...` to stage runtime-base into the artifact before deploy."
	case errors.Is(err, artifactcore.ErrSecretEnvFileRequired), errors.As(err, &missingSecretKeys):
		return "set `--secret-env <path>` with all required secret keys listed in artifact.yml."
	case errors.As(err, &missingReferencedPath):
		return "confirm `--artifact` and referenced files exist and are readable."
	default:
		return "run `artifactctl deploy --help` for required arguments."
	}
}
