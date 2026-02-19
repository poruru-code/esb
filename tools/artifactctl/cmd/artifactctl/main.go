package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

const usageMessage = "usage: artifactctl <validate-id|merge|prepare-images|apply> [flags]"

type commandDeps struct {
	validateIDs   func(string) error
	mergeConfig   func(artifactcore.MergeRequest) error
	prepareImages func(artifactcore.PrepareImagesRequest) error
	apply         func(artifactcore.ApplyRequest) error
	warningWriter io.Writer
	helpWriter    io.Writer
}

func main() {
	if err := run(os.Args[1:], defaultDeps()); err != nil {
		exitf("%v", err)
	}
}

func defaultDeps() commandDeps {
	return commandDeps{
		validateIDs:   artifactcore.ValidateIDs,
		mergeConfig:   artifactcore.MergeRuntimeConfig,
		prepareImages: artifactcore.PrepareImages,
		apply:         artifactcore.Apply,
		warningWriter: os.Stderr,
		helpWriter:    os.Stdout,
	}
}

func run(args []string, deps commandDeps) error {
	if len(args) < 1 {
		return fmt.Errorf(usageMessage)
	}
	switch args[0] {
	case "validate-id":
		return runValidateID(args[1:], deps)
	case "merge":
		return runMerge(args[1:], deps)
	case "prepare-images":
		return runPrepareImages(args[1:], deps)
	case "apply":
		return runApply(args[1:], deps)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func handleParseError(err error, fs *flag.FlagSet, deps commandDeps) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, flag.ErrHelp) {
		helpWriter := deps.helpWriter
		if helpWriter == nil {
			helpWriter = os.Stdout
		}
		fs.SetOutput(helpWriter)
		fs.Usage()
		return nil
	}
	return err
}

func runValidateID(args []string, deps commandDeps) error {
	fs := newFlagSet("validate-id")
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	if err := fs.Parse(args); err != nil {
		return handleParseError(err, fs, deps)
	}
	if *artifact == "" {
		return fmt.Errorf("--artifact is required")
	}
	if err := deps.validateIDs(*artifact); err != nil {
		return fmt.Errorf("validate-id failed: %w", err)
	}
	return nil
}

func runMerge(args []string, deps commandDeps) error {
	fs := newFlagSet("merge")
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	out := fs.String("out", "", "Output CONFIG_DIR")
	if err := fs.Parse(args); err != nil {
		return handleParseError(err, fs, deps)
	}
	if *artifact == "" {
		return fmt.Errorf("--artifact is required")
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	if err := deps.mergeConfig(artifactcore.MergeRequest{ArtifactPath: *artifact, OutputDir: *out}); err != nil {
		return fmt.Errorf("merge failed: %w", err)
	}
	return nil
}

func runApply(args []string, deps commandDeps) error {
	fs := newFlagSet("apply")
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	out := fs.String("out", "", "Output CONFIG_DIR")
	secretEnv := fs.String("secret-env", "", "Path to secret env file")
	strict := fs.Bool("strict", false, "Enable strict runtime metadata validation")
	if err := fs.Parse(args); err != nil {
		return handleParseError(err, fs, deps)
	}
	if *artifact == "" {
		return fmt.Errorf("--artifact is required")
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	warningWriter := deps.warningWriter
	if warningWriter == nil {
		warningWriter = os.Stderr
	}
	req := artifactcore.ApplyRequest{
		ArtifactPath:  *artifact,
		OutputDir:     *out,
		SecretEnvPath: *secretEnv,
		Strict:        *strict,
		WarningWriter: warningWriter,
	}
	if err := deps.apply(req); err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}
	return nil
}

func runPrepareImages(args []string, deps commandDeps) error {
	fs := newFlagSet("prepare-images")
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	noCache := fs.Bool("no-cache", false, "Do not use cache when building images")
	if err := fs.Parse(args); err != nil {
		return handleParseError(err, fs, deps)
	}
	if *artifact == "" {
		return fmt.Errorf("--artifact is required")
	}
	req := artifactcore.PrepareImagesRequest{
		ArtifactPath: *artifact,
		NoCache:      *noCache,
	}
	if err := deps.prepareImages(req); err != nil {
		return fmt.Errorf("prepare-images failed: %w", err)
	}
	return nil
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
