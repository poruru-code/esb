// Where: cli/internal/infra/templategen/registry_helpers.go
// What: Registry endpoint helpers for image import manifest generation.
// Why: Keep templategen independent from infra/build orchestration internals.
package templategen

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
)

func resolveHostRegistryAddress() (string, bool) {
	if value := strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")); value != "" {
		return strings.TrimPrefix(value, "http://"), true
	}
	port := strings.TrimSpace(os.Getenv(constants.EnvPortRegistry))
	if port == "" {
		port = "5010"
	}
	return fmt.Sprintf("127.0.0.1:%s", port), false
}
