package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/engine"
)

func main() {
	if len(os.Args) < 2 {
		exitf("usage: artifactctl <validate-id|merge|apply> [flags]")
	}
	cmd := os.Args[1]
	switch cmd {
	case "validate-id":
		runValidateID(os.Args[2:])
	case "merge":
		runMerge(os.Args[2:])
	case "apply":
		runApply(os.Args[2:])
	default:
		exitf("unknown command: %s", cmd)
	}
}

func runValidateID(args []string) {
	fs := flag.NewFlagSet("validate-id", flag.ExitOnError)
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	_ = fs.Parse(args)
	if *artifact == "" {
		exitf("--artifact is required")
	}
	if err := engine.ValidateIDs(*artifact); err != nil {
		exitf("validate-id failed: %v", err)
	}
}

func runMerge(args []string) {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	out := fs.String("out", "", "Output CONFIG_DIR")
	_ = fs.Parse(args)
	if *artifact == "" {
		exitf("--artifact is required")
	}
	if *out == "" {
		exitf("--out is required")
	}
	if err := engine.MergeRuntimeConfig(engine.MergeRequest{ArtifactPath: *artifact, OutputDir: *out}); err != nil {
		exitf("merge failed: %v", err)
	}
}

func runApply(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	artifact := fs.String("artifact", "", "Path to artifact.yml")
	out := fs.String("out", "", "Output CONFIG_DIR")
	secretEnv := fs.String("secret-env", "", "Path to secret env file")
	strict := fs.Bool("strict", false, "Enable strict runtime metadata validation")
	_ = fs.Parse(args)
	if *artifact == "" {
		exitf("--artifact is required")
	}
	if *out == "" {
		exitf("--out is required")
	}
	req := engine.ApplyRequest{
		ArtifactPath:  *artifact,
		OutputDir:     *out,
		SecretEnvPath: *secretEnv,
		Strict:        *strict,
		WarningWriter: os.Stderr,
	}
	if err := engine.Apply(req); err != nil {
		exitf("apply failed: %v", err)
	}
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
