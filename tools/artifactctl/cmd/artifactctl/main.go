package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/poruru-code/esb/pkg/artifactcore"
	"github.com/poruru-code/esb/pkg/composeprovision"
	"github.com/poruru-code/esb/pkg/deployops"
	"github.com/poruru-code/esb/pkg/deployops/mavenshim"
)

type CLI struct {
	Deploy    DeployCmd    `cmd:"" help:"Prepare images and apply artifact manifest"`
	Provision ProvisionCmd `cmd:"" help:"Run deploy provisioner via docker compose"`
	Internal  InternalCmd  `cmd:"" help:"Internal commands for orchestrators"`
}

type DeployCmd struct {
	Artifact string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	NoCache  bool   `name:"no-cache" help:"Do not use cache when building images"`
}

type ProvisionCmd struct {
	ComposeProject string   `name:"project" required:"" help:"Compose project name"`
	ComposeFiles   []string `name:"compose-file" required:"" sep:"," help:"Compose file(s) to use (repeatable or comma-separated)"`
	EnvFile        string   `name:"env-file" help:"Path to compose env file"`
	ProjectDir     string   `name:"project-dir" help:"Working directory for docker compose (default: current directory)"`
	WithDeps       bool     `name:"with-deps" help:"Start dependent services when running provisioner"`
	Verbose        bool     `short:"v" help:"Verbose output"`
}

type InternalCmd struct {
	MavenShim    InternalMavenShimCmd    `cmd:"" name:"maven-shim" help:"Maven shim helper operations"`
	FixtureImage InternalFixtureImageCmd `cmd:"" name:"fixture-image" help:"Local fixture image helper operations"`
	Capabilities InternalCapabilitiesCmd `cmd:"" name:"capabilities" help:"Print internal contract versions for orchestrators"`
}

type InternalMavenShimCmd struct {
	Ensure InternalMavenShimEnsureCmd `cmd:"" name:"ensure" help:"Ensure a Maven shim image and print JSON payload"`
}

type InternalMavenShimEnsureCmd struct {
	BaseImage    string `name:"base-image" required:"" help:"Base Maven image reference used for shim derivation"`
	HostRegistry string `name:"host-registry" help:"Host registry prefix (for pushable shim reference)"`
	NoCache      bool   `name:"no-cache" help:"Do not use local cache when building shim image"`
	Output       string `name:"output" default:"json" enum:"json" help:"Output format"`
}

type InternalFixtureImageCmd struct {
	Ensure InternalFixtureImageEnsureCmd `cmd:"" name:"ensure" help:"Ensure local fixture images and print JSON payload"`
}

type InternalFixtureImageEnsureCmd struct {
	Artifact string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	NoCache  bool   `name:"no-cache" help:"Do not use cache when building fixture images"`
	Output   string `name:"output" default:"json" enum:"json" help:"Output format"`
}

type InternalCapabilitiesCmd struct {
	Output string `name:"output" default:"json" enum:"json" help:"Output format"`
}

type kongExitCode int

type commandDeps struct {
	executeDeploy       func(deployops.Input) (artifactcore.ApplyResult, error)
	executeProvision    func(ProvisionInput) error
	ensureMavenShim     func(MavenShimEnsureInput) (MavenShimEnsureResult, error)
	ensureFixtureImages func(FixtureImageEnsureInput) (FixtureImageEnsureResult, error)
	capabilities        func() ArtifactctlCapabilities
	warningWriter       io.Writer
	out                 io.Writer
	errOut              io.Writer
}

type ProvisionInput struct {
	ComposeProject string
	ComposeFiles   []string
	EnvFile        string
	ProjectDir     string
	NoDeps         bool
	Verbose        bool
}

type MavenShimEnsureInput struct {
	BaseImage    string
	HostRegistry string
	NoCache      bool
}

type MavenShimEnsureResult struct {
	SchemaVersion int    `json:"schema_version"`
	ShimImage     string `json:"shim_image"`
}

type ArtifactctlCapabilities struct {
	SchemaVersion int                         `json:"schema_version"`
	Contracts     ArtifactctlContractVersions `json:"contracts"`
}

type ArtifactctlContractVersions struct {
	MavenShimEnsureSchemaVersion    int `json:"maven_shim_ensure_schema_version"`
	FixtureImageEnsureSchemaVersion int `json:"fixture_image_ensure_schema_version"`
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps()))
}

func defaultDeps() commandDeps {
	return commandDeps{
		executeDeploy:       deployops.Execute,
		executeProvision:    executeProvision,
		ensureMavenShim:     executeMavenShimEnsure,
		ensureFixtureImages: executeFixtureImageEnsure,
		capabilities:        currentCapabilities,
		warningWriter:       os.Stderr,
		out:                 os.Stdout,
		errOut:              os.Stderr,
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
	case "internal maven-shim ensure":
		if err := runInternalMavenShimEnsure(cli.Internal.MavenShim.Ensure, deps, out, errOut); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl internal maven-shim ensure --help`.")
			return 1
		}
		return 0
	case "internal fixture-image ensure":
		if err := runInternalFixtureImageEnsure(cli.Internal.FixtureImage.Ensure, deps, out, errOut); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl internal fixture-image ensure --help`.")
			return 1
		}
		return 0
	case "internal capabilities":
		if err := runInternalCapabilities(cli.Internal.Capabilities, deps, out); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl internal capabilities --help`.")
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
		executeDeploy = deployops.Execute
	}
	warningWriter := deps.warningWriter
	if warningWriter == nil {
		if errOut == nil {
			warningWriter = os.Stderr
		} else {
			warningWriter = errOut
		}
	}
	result, err := executeDeploy(deployops.Input{
		ArtifactPath: cmd.Artifact,
		NoCache:      cmd.NoCache,
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

func runInternalMavenShimEnsure(
	cmd InternalMavenShimEnsureCmd,
	deps commandDeps,
	out io.Writer,
	errOut io.Writer,
) error {
	ensureMavenShim := deps.ensureMavenShim
	var (
		result MavenShimEnsureResult
		err    error
	)
	if ensureMavenShim != nil {
		result, err = ensureMavenShim(MavenShimEnsureInput{
			BaseImage:    cmd.BaseImage,
			HostRegistry: cmd.HostRegistry,
			NoCache:      cmd.NoCache,
		})
	} else {
		result, err = executeMavenShimEnsureWithLogWriter(MavenShimEnsureInput{
			BaseImage:    cmd.BaseImage,
			HostRegistry: cmd.HostRegistry,
			NoCache:      cmd.NoCache,
		}, errOut)
	}
	if err != nil {
		return fmt.Errorf("maven shim ensure failed: %w", err)
	}
	if strings.TrimSpace(cmd.Output) != "json" {
		return fmt.Errorf("unsupported output format: %s", cmd.Output)
	}
	encoder := json.NewEncoder(out)
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode maven shim ensure output: %w", err)
	}
	return nil
}

func runInternalFixtureImageEnsure(
	cmd InternalFixtureImageEnsureCmd,
	deps commandDeps,
	out io.Writer,
	errOut io.Writer,
) error {
	ensureFixtureImages := deps.ensureFixtureImages
	var (
		result FixtureImageEnsureResult
		err    error
	)
	if ensureFixtureImages != nil {
		result, err = ensureFixtureImages(FixtureImageEnsureInput{
			ArtifactPath: cmd.Artifact,
			NoCache:      cmd.NoCache,
		})
	} else {
		result, err = executeFixtureImageEnsureWithLogWriter(FixtureImageEnsureInput{
			ArtifactPath: cmd.Artifact,
			NoCache:      cmd.NoCache,
		}, errOut)
	}
	if err != nil {
		return fmt.Errorf("fixture image ensure failed: %w", err)
	}
	if strings.TrimSpace(cmd.Output) != "json" {
		return fmt.Errorf("unsupported output format: %s", cmd.Output)
	}
	encoder := json.NewEncoder(out)
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode fixture image ensure output: %w", err)
	}
	return nil
}

func runInternalCapabilities(
	cmd InternalCapabilitiesCmd,
	deps commandDeps,
	out io.Writer,
) error {
	if strings.TrimSpace(cmd.Output) != "json" {
		return fmt.Errorf("unsupported output format: %s", cmd.Output)
	}
	capabilitiesProvider := deps.capabilities
	if capabilitiesProvider == nil {
		capabilitiesProvider = currentCapabilities
	}
	encoder := json.NewEncoder(out)
	if err := encoder.Encode(capabilitiesProvider()); err != nil {
		return fmt.Errorf("encode capabilities output: %w", err)
	}
	return nil
}

func executeMavenShimEnsure(input MavenShimEnsureInput) (MavenShimEnsureResult, error) {
	return executeMavenShimEnsureWithLogWriter(input, os.Stderr)
}

func executeMavenShimEnsureWithLogWriter(
	input MavenShimEnsureInput,
	logWriter io.Writer,
) (MavenShimEnsureResult, error) {
	if logWriter == nil {
		logWriter = os.Stderr
	}
	result, err := mavenshim.EnsureImage(mavenshim.EnsureInput{
		BaseImage:    input.BaseImage,
		HostRegistry: input.HostRegistry,
		NoCache:      input.NoCache,
		Runner:       stderrCommandRunner{writer: logWriter},
	})
	if err != nil {
		return MavenShimEnsureResult{}, err
	}
	return MavenShimEnsureResult{
		SchemaVersion: 1,
		ShimImage:     result.ShimImage,
	}, nil
}

func currentCapabilities() ArtifactctlCapabilities {
	return ArtifactctlCapabilities{
		SchemaVersion: 1,
		Contracts: ArtifactctlContractVersions{
			MavenShimEnsureSchemaVersion:    1,
			FixtureImageEnsureSchemaVersion: fixtureImageEnsureSchemaVersion,
		},
	}
}

type stderrCommandRunner struct {
	writer io.Writer
}

func (r stderrCommandRunner) Run(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command is empty")
	}
	writer := r.writer
	if writer == nil {
		writer = os.Stderr
	}
	command := exec.Command(cmd[0], cmd[1:]...)
	command.Stdout = writer
	command.Stderr = writer
	return command.Run()
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
	var missingReferencedPath artifactcore.MissingReferencedPathError

	switch {
	case errors.As(err, &missingReferencedPath):
		return "confirm `--artifact` and referenced files exist and are readable."
	default:
		return "run `artifactctl deploy --help` for required arguments."
	}
}
