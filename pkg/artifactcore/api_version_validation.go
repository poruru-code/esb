package artifactcore

import (
	"fmt"
	"strconv"
	"strings"
)

func validateAPIVersion(field, actual, expected string, warnings *[]string) error {
	got := strings.TrimSpace(actual)
	if got == "" {
		return nil
	}
	want := strings.TrimSpace(expected)
	if want == "" {
		return fmt.Errorf("%s expected version is not configured", field)
	}

	gotMajor, gotMinor, err := parseAPIVersion(got)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s is invalid (%q): %v", field, got, err))
		return nil
	}
	wantMajor, wantMinor, err := parseAPIVersion(want)
	if err != nil {
		return fmt.Errorf("supported api_version for %s is invalid (%q): %w", field, want, err)
	}

	if gotMajor != wantMajor {
		return fmt.Errorf("%s major mismatch: got %q, supported %q", field, got, want)
	}
	if gotMinor != wantMinor {
		message := fmt.Sprintf("%s minor mismatch: got %q, supported %q", field, got, want)
		*warnings = append(*warnings, message)
	}
	return nil
}

func parseAPIVersion(value string) (int, int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, 0, fmt.Errorf("empty version")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, 0, fmt.Errorf("expected major.minor format")
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, fmt.Errorf("invalid major")
	}
	minor := 0
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil || minor < 0 {
			return 0, 0, fmt.Errorf("invalid minor")
		}
	}
	return major, minor, nil
}
