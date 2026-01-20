// Where: cli/internal/commands/output.go
// What: Output helpers for command adapters.
// Why: Centralize UserInterface usage and raw line output.
package commands

import (
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

func legacyUI(out io.Writer) ports.UserInterface {
	return ports.NewLegacyUI(out)
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
