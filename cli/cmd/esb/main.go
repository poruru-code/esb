// Where: cli/cmd/esb/main.go
// What: CLI entrypoint.
// Why: Execute ESB commands with configured dependencies.
package main

import (
	"fmt"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
)

func main() {
	deps, closer, err := buildDependencies()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if closer != nil {
		defer closer.Close()
	}

	os.Exit(app.Run(os.Args[1:], deps))
}
