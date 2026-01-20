// Where: cli/internal/helpers/mode.go
// What: Runtime mode environment helpers.
// Why: Keep mode propagation consistent across commands.
package helpers

import (
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

// applyModeEnv sets the mode environment variable if not already set.
// This ensures consistent mode propagation across all CLI commands.
func applyModeEnv(mode string) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return
	}
	if strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixMode)) != "" {
		return
	}
	envutil.SetHostEnv(constants.HostSuffixMode, strings.ToLower(trimmed))
}
