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
// Example: HostEnvKey("ENV") returns "APP_ENV" when ENV_PREFIX=APP.
func HostEnvKey(suffix string) (string, error) {
	prefix := strings.TrimSpace(os.Getenv("ENV_PREFIX"))
	if prefix == "" {
		return "", errEnvPrefixRequired
	}
	return prefix + "_" + suffix, nil
}

// GetHostEnv retrieves a host-level environment variable.
// Example: GetHostEnv("ENV") returns the value of APP_ENV.
func GetHostEnv(suffix string) (string, error) {
	key, err := HostEnvKey(suffix)
	if err != nil {
		return "", err
	}
	return os.Getenv(key), nil
}

// SetHostEnv sets a host-level environment variable.
// Example: SetHostEnv("ENV", "production") sets APP_ENV=production.
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

func hostEnvKeyIfConfigured(suffix string) (string, bool, error) {
	prefix := strings.TrimSpace(os.Getenv("ENV_PREFIX"))
	if prefix == "" {
		return "", false, nil
	}
	return prefix + "_" + suffix, true, nil
}

// GetCompatEnv resolves a value from canonical and prefixed keys.
// Resolution order:
//  1. canonical key (for example TAG)
//  2. ENV_PREFIX-derived key (for example APP_TAG)
//
// It returns the value and the key it came from.
func GetCompatEnv(suffix, canonicalKey string) (string, string) {
	trimmedCanonical := strings.TrimSpace(canonicalKey)
	if trimmedCanonical != "" {
		if value := strings.TrimSpace(os.Getenv(trimmedCanonical)); value != "" {
			return value, trimmedCanonical
		}
	}

	if hostKey, ok, err := hostEnvKeyIfConfigured(suffix); err == nil && ok {
		if value := strings.TrimSpace(os.Getenv(hostKey)); value != "" {
			return value, hostKey
		}
	}
	return "", ""
}

// SetCompatEnv sets canonical and ENV_PREFIX-derived keys.
func SetCompatEnv(suffix, canonicalKey, value string) error {
	written := false
	trimmedCanonical := strings.TrimSpace(canonicalKey)
	if trimmedCanonical != "" {
		if err := os.Setenv(trimmedCanonical, value); err != nil {
			return fmt.Errorf("set env %s: %w", trimmedCanonical, err)
		}
		written = true
	}

	hostKey, ok, err := hostEnvKeyIfConfigured(suffix)
	if err != nil {
		return err
	}
	if ok {
		if err := os.Setenv(hostKey, value); err != nil {
			return fmt.Errorf("set env %s: %w", hostKey, err)
		}
		written = true
	}
	if !written {
		return errEnvPrefixRequired
	}
	return nil
}
