// Package envutil provides helper functions for environment variable handling.
package envutil

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var errEnvPrefixRequired = errors.New("ENV_PREFIX is required")

// HostEnvKey constructs a host-level environment variable name.
// by combining ENV_PREFIX with the given suffix.
// Example: HostEnvKey("ENV") returns "ESB_ENV" when ENV_PREFIX=ESB.
func HostEnvKey(suffix string) (string, error) {
	prefix := strings.TrimSpace(os.Getenv("ENV_PREFIX"))
	if prefix == "" {
		return "", errEnvPrefixRequired
	}
	return prefix + "_" + suffix, nil
}

// GetHostEnv retrieves a host-level environment variable.
// Example: GetHostEnv("ENV") returns the value of ESB_ENV.
func GetHostEnv(suffix string) (string, error) {
	key, err := HostEnvKey(suffix)
	if err != nil {
		return "", err
	}
	return os.Getenv(key), nil
}

// SetHostEnv sets a host-level environment variable.
// Example: SetHostEnv("ENV", "production") sets ESB_ENV=production.
func SetHostEnv(suffix, value string) error {
	key, err := HostEnvKey(suffix)
	if err != nil {
		return err
	}
	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("set env %s: %w", key, err)
	}
	return nil
}
