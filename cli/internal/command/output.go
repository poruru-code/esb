// Where: cli/internal/command/output.go
// What: Output helpers for command adapters.
// Why: Centralize UserInterface usage and raw line output.
package command

import (
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
)

func legacyUI(out io.Writer) ui.UserInterface {
	return ui.NewLegacyUI(out)
}

func writeString(out io.Writer, text string) {
	if out == nil || text == "" {
		return
	}
	_, _ = io.WriteString(out, text)
}

func writeLine(out io.Writer, line string) {
	if out == nil {
		return
	}
	if strings.HasSuffix(line, "\n") {
		_, _ = io.WriteString(out, line)
		return
	}
	_, _ = io.WriteString(out, line+"\n")
}
