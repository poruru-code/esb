// Where: cli/internal/command/output.go
// What: Output helpers for command adapters.
// Why: Centralize UserInterface usage and raw line output.
package command

import (
	"fmt"
	"io"
	"os"

	"github.com/poruru-code/esb/cli/internal/infra/ui"
)

func legacyUI(out io.Writer) ui.UserInterface {
	return ui.NewLegacyUI(out)
}

func resolveErrWriter(out io.Writer) io.Writer {
	if out != nil {
		return out
	}
	return os.Stderr
}

func writeWarningf(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(resolveErrWriter(out), format, args...)
}
