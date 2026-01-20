// Where: cli/internal/commands/resolve_options.go
// What: Shared resolution options for project/env selection.
// Why: Keep force/interactive behavior consistent across commands.
package commands

import (
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type resolveOptions struct {
	Force           bool
	Interactive     bool
	Prompt          state.PromptFunc
	AllowMissingEnv bool
}

func newResolveOptions(force bool) resolveOptions {
	return resolveOptions{
		Force:       force,
		Interactive: interaction.IsTerminal(os.Stdin),
		Prompt:      interaction.PromptYesNo,
	}
}
