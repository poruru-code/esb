// Package envutil provides helper functions for environment variable handling.
package envutil

import (
	"fmt"
	"os"
	"strings"
)

// HostEnvKey constructs a host-level environment variable name
// by combining ENV_PREFIX with the given suffix.
// Example: HostEnvKey("ENV") returns "ESB_ENV" when ENV_PREFIX=ESB
func HostEnvKey(suffix string) (string, error) {
	prefix := strings.TrimSpace(os.Getenv("ENV_PREFIX"))
	if prefix == "" {
		return "", fmt.Errorf("ENV_PREFIX is required")
	}
	return prefix + "_" + suffix, nil
}

// GetHostEnv retrieves a host-level environment variable.
// Example: GetHostEnv("ENV") returns the value of ESB_ENV
func GetHostEnv(suffix string) (string, error) {
	key, err := HostEnvKey(suffix)
	if err != nil {
		return "", err
	}
	return os.Getenv(key), nil
}

// SetHostEnv sets a host-level environment variable.
// Example: SetHostEnv("ENV", "production") sets ESB_ENV=production
func SetHostEnv(suffix, value string) error {
	key, err := HostEnvKey(suffix)
	if err != nil {
		return err
	}
	return os.Setenv(key, value)
}
