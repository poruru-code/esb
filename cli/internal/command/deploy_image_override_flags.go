// Where: cli/internal/command/deploy_image_override_flags.go
// What: Helpers for parsing per-function deploy override flags.
// Why: Keep key/value parsing and filtering separate from deploy orchestration.
package command

import (
	"fmt"
	"strings"
)

func parseFunctionOverrideFlag(values []string, flagName string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return nil, fmt.Errorf("%s must be in <function>=<value> format: %q", flagName, raw)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("%s must be in <function>=<value> format: %q", flagName, raw)
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func filterFunctionOverrides(overrides map[string]string, functionNames []string) map[string]string {
	if len(overrides) == 0 || len(functionNames) == 0 {
		return nil
	}
	selected := map[string]string{}
	for _, name := range functionNames {
		if value, ok := overrides[name]; ok {
			selected[name] = value
		}
	}
	if len(selected) == 0 {
		return nil
	}
	return selected
}
