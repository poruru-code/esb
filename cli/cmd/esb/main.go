// Where: cli/cmd/esb/main.go
// What: CLI entrypoint.
// Why: Execute ESB commands with configured dependencies.
package main

import (
	"fmt"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/wire"
)

// main is the entry point for the ESB CLI. It builds the dependencies,
// parses command-line arguments, and dispatches to the appropriate command handler.
func main() {
	deps, closer, err := wire.BuildDependencies()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if closer != nil {
		defer closer.Close()
	}

	os.Exit(app.Run(os.Args[1:], deps))
}
