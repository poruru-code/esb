// Where: cli/internal/infra/build/merge_config_helpers.go
// What: small helper utilities for merge config logic.
// Why: Keep reusable pure helpers out of entry/merge implementation files.
package build

import "strings"

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
