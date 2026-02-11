// Where: cli/internal/infra/templategen/output.go
// What: Output writer resolution helpers for generator logging.
// Why: Keep stdout/stderr defaults centralized while allowing writer injection.
package templategen

import (
	"io"
	"os"
)

func resolveGenerateOutput(out io.Writer) io.Writer {
	if out != nil {
		return out
	}
	return os.Stdout
}

func resolveGenerateErrOutput(out io.Writer) io.Writer {
	if out != nil {
		return out
	}
	return os.Stderr
}
