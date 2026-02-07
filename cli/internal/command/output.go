// Where: cli/internal/command/output.go
// What: Output helpers for command adapters.
// Why: Centralize UserInterface usage and raw line output.
package command

import (
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
)

func legacyUI(out io.Writer) ui.UserInterface {
	return ui.NewLegacyUI(out)
}
