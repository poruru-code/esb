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
	"github.com/poruru/edge-serverless-box/pkg/composeprovision"
	"github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/deployops"
)

type CLI struct {
	Deploy    DeployCmd    `cmd:"" help:"Prepare images and apply artifact manifest"`
	Provision ProvisionCmd `cmd:"" help:"Run deploy provisioner via docker compose"`
	Manifest  ManifestCmd  `cmd:"" help:"Artifact manifest maintenance helpers"`
}

type DeployCmd struct {
	Artifact  string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	Output    string `name:"out" required:"" help:"Output config directory (CONFIG_DIR)"`
	SecretEnv string `name:"secret-env" help:"Path to secret env file"`
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

type ManifestCmd struct {
	SyncIDs ManifestSyncIDsCmd `cmd:"" name:"sync-ids" help:"Recompute deterministic IDs from source_template values"`
}

type ManifestSyncIDsCmd struct {
	Artifact string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	Check    bool   `name:"check" help:"Check only (do not write). Exit non-zero if updates are needed"`
}

type kongExitCode int

type commandDeps struct {
	executeDeploy    func(deployops.Input) (artifactcore.ApplyResult, error)
	executeProvision func(ProvisionInput) error
	syncManifestIDs  func(path string, write bool) (int, error)
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
		executeDeploy:    deployops.Execute,
		executeProvision: executeProvision,
		syncManifestIDs:  syncManifestIDs,
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
		_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl --help`, `artifactctl deploy --help`, `artifactctl provision --help`, or `artifactctl manifest sync-ids --help`.")
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
	case "manifest sync-ids":
		if err := runManifestSyncIDs(cli.Manifest.SyncIDs, deps, out); err != nil {
			_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
			_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl manifest sync-ids --help` for required arguments.")
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
		ArtifactPath:  cmd.Artifact,
		OutputDir:     cmd.Output,
		SecretEnvPath: cmd.SecretEnv,
		NoCache:       cmd.NoCache,
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

func runManifestSyncIDs(cmd ManifestSyncIDsCmd, deps commandDeps, out io.Writer) error {
	syncIDs := deps.syncManifestIDs
	if syncIDs == nil {
		syncIDs = syncManifestIDs
	}

	changed, err := syncIDs(cmd.Artifact, !cmd.Check)
	if err != nil {
		return err
	}
	if cmd.Check {
		if changed > 0 {
			return fmt.Errorf("artifact manifest requires id sync: %d entrie(s) differ", changed)
		}
		_, _ = fmt.Fprintln(out, "artifact manifest ids are already synchronized")
		return nil
	}
	_, _ = fmt.Fprintf(out, "artifact manifest id sync complete: updated=%d\n", changed)
	return nil
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
	case errors.Is(err, artifactcore.ErrSecretEnvFileRequired), errors.As(err, &missingSecretKeys):
		return "set `--secret-env <path>` with all required secret keys listed in artifact.yml."
	case errors.As(err, &missingReferencedPath):
		return "confirm `--artifact` and referenced files exist and are readable."
	default:
		return "run `artifactctl deploy --help` for required arguments."
	}
}

func syncManifestIDs(path string, write bool) (int, error) {
	manifest, err := artifactcore.ReadArtifactManifestUnchecked(path)
	if err != nil {
		return 0, err
	}
	changed := artifactcore.SyncArtifactIDs(&manifest)
	if err := manifest.Validate(); err != nil {
		return changed, err
	}
	if !write {
		return changed, nil
	}
	if err := artifactcore.WriteArtifactManifest(path, manifest); err != nil {
		return changed, err
	}
	return changed, nil
}
