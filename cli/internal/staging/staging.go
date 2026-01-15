// Where: cli/internal/staging/staging.go
// What: Shared helpers for ESB staging directory layout.
// Why: Keep builder and runtime components aligned on where staged configs land.
package staging

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ComposeProjectKey returns a filesystem-safe staging key for the provided
// compose project, falling back to a predictable value when the input is empty.
func ComposeProjectKey(composeProject, env string) string {
	if key := strings.TrimSpace(composeProject); key != "" {
		return key
	}
	if env = strings.TrimSpace(env); env != "" {
		return fmt.Sprintf("esb-%s", strings.ToLower(env))
	}
	return "esb"
}

// BaseDir returns the absolute staging directory inside services/gateway/.esb-staging.
func BaseDir(repoRoot, composeProject, env string) string {
	return filepath.Join(repoRoot, "services", "gateway", ".esb-staging", ComposeProjectKey(composeProject, env))
}

// ConfigDirRelative returns the relative staging config directory used by runtime code.
func ConfigDirRelative(composeProject, env string) string {
	return filepath.Join("services", "gateway", ".esb-staging", ComposeProjectKey(composeProject, env), env, "config")
}
