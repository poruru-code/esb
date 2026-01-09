// Where: cli/internal/app/mode.go
// What: Runtime mode environment helpers.
// Why: Keep mode propagation consistent across commands.
package app

import (
	"os"
	"strings"
)

func applyModeEnv(mode string) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return
	}
	if strings.TrimSpace(os.Getenv("ESB_MODE")) != "" {
		return
	}
	_ = os.Setenv("ESB_MODE", strings.ToLower(trimmed))
}
