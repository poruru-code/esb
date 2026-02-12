// Where: cli/internal/helpers/mode.go
// What: Runtime mode environment helpers.
// Why: Keep mode propagation consistent across commands.
package env

import (
	"fmt"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

// applyModeEnv sets the mode environment variable if not already set.
// This ensures consistent mode propagation across all CLI commands.
func applyModeEnv(mode string) error {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return nil
	}
	existing, _ := envutil.GetCompatEnv(constants.HostSuffixMode, "MODE")
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	if err := envutil.SetCompatEnv(constants.HostSuffixMode, "MODE", strings.ToLower(trimmed)); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixMode, err)
	}
	return nil
}
