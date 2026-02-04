// Where: cli/cmd/esb/main.go
// What: CLI entrypoint.
// Why: Execute ESB commands with configured dependencies.
package main

import (
	"fmt"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/command"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/env"
)

// main is the entry point for the ESB CLI. It builds the dependencies,
// parses command-line arguments, and dispatches to the appropriate command handler.
func main() {
	if err := env.ApplyBrandingEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	args := os.Args[1:]
	deps, closer, err := app.BuildDependencies(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exitCode := command.Run(args, deps)
	if closer != nil {
		if err := closer.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	os.Exit(exitCode)
}
