// Where: cli/internal/app/resolve_options.go
// What: Shared resolution options for project/env selection.
// Why: Keep force/interactive behavior consistent across commands.
package app

import (
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type resolveOptions struct {
	Force       bool
	Interactive bool
	Prompt      state.PromptFunc
}

func newResolveOptions(force bool) resolveOptions {
	return resolveOptions{
		Force:       force,
		Interactive: isTerminal(os.Stdin),
		Prompt:      promptYesNo,
	}
}
