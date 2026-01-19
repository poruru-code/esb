// Package envutil provides helper functions for environment variable handling.
package envutil

import (
	"os"
	"strings"
)

// HostEnvKey constructs a host-level environment variable name
// by combining ENV_PREFIX with the given suffix.
// Example: HostEnvKey("ENV") returns "ESB_ENV" when ENV_PREFIX=ESB
func HostEnvKey(suffix string) string {
	prefix := strings.TrimSpace(os.Getenv("ENV_PREFIX"))
	if prefix == "" {
		prefix = "ESB" // Fallback default
	}
	return prefix + "_" + suffix
}

// GetHostEnv retrieves a host-level environment variable.
// Example: GetHostEnv("ENV") returns the value of ESB_ENV
func GetHostEnv(suffix string) string {
	return os.Getenv(HostEnvKey(suffix))
}

// SetHostEnv sets a host-level environment variable.
// Example: SetHostEnv("ENV", "production") sets ESB_ENV=production
func SetHostEnv(suffix, value string) {
	_ = os.Setenv(HostEnvKey(suffix), value)
}
