package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

type CLI struct {
	Deploy DeployCmd `cmd:"" help:"Prepare images and apply artifact manifest"`
}

type DeployCmd struct {
	Artifact  string `name:"artifact" required:"" help:"Path to artifact manifest (artifact.yml)"`
	Output    string `name:"out" required:"" help:"Output config directory (CONFIG_DIR)"`
	SecretEnv string `name:"secret-env" help:"Path to secret env file"`
	Strict    bool   `name:"strict" help:"Enable strict runtime metadata validation"`
	NoCache   bool   `name:"no-cache" help:"Do not use cache when building images"`
}

type kongExitCode int

type commandDeps struct {
	prepareImages func(artifactcore.PrepareImagesRequest) error
	apply         func(artifactcore.ApplyRequest) error
	warningWriter io.Writer
	out           io.Writer
	errOut        io.Writer
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps()))
}

func defaultDeps() commandDeps {
	return commandDeps{
		prepareImages: artifactcore.PrepareImages,
		apply:         artifactcore.Apply,
		warningWriter: os.Stderr,
		out:           os.Stdout,
		errOut:        os.Stderr,
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
		_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl --help` or `artifactctl deploy --help`.")
		return 1
	}
	if ctx.Command() != "deploy" {
		_, _ = fmt.Fprintf(errOut, "Error: unsupported command: %s\n", ctx.Command())
		_, _ = fmt.Fprintln(errOut, "Hint: run `artifactctl --help`.")
		return 1
	}
	if err := runDeploy(cli.Deploy, deps, errOut); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		if hint := hintForDeployError(err); hint != "" {
			_, _ = fmt.Fprintf(errOut, "Hint: %s\n", hint)
		}
		return 1
	}
	return 0
}

func runDeploy(cmd DeployCmd, deps commandDeps, errOut io.Writer) error {
	artifactPath := strings.TrimSpace(cmd.Artifact)
	outputDir := strings.TrimSpace(cmd.Output)
	if err := deps.prepareImages(artifactcore.PrepareImagesRequest{
		ArtifactPath: artifactPath,
		NoCache:      cmd.NoCache,
	}); err != nil {
		return fmt.Errorf("deploy failed during image preparation: %w", err)
	}
	warningWriter := deps.warningWriter
	if warningWriter == nil {
		if errOut == nil {
			warningWriter = os.Stderr
		} else {
			warningWriter = errOut
		}
	}
	applyReq := artifactcore.NewApplyRequest(
		artifactPath,
		outputDir,
		cmd.SecretEnv,
		cmd.Strict,
		warningWriter,
	)
	if err := deps.apply(applyReq); err != nil {
		return fmt.Errorf("deploy failed during artifact apply: %w", err)
	}
	return nil
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
