// Where: cli/internal/generator/parser_helpers.go
// What: Helper utilities for SAM parsing.
// Why: Keep parser logic focused on SAM semantics.
package generator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if v, ok := value.([]any); ok {
		return v
	}
	return nil
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func asStringDefault(value any, fallback string) string {
	if out := asString(value); out != "" {
		return out
	}
	return fallback
}

func asIntPointer(value any) (*int, bool) {
	switch typed := value.(type) {
	case int:
		return &typed, true
	case int64:
		intVal := int(typed)
		return &intVal, true
	case float64:
		intVal := int(typed)
		return &intVal, true
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return &parsed, true
		}
	}
	return nil, false
}

var subPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

func resolveIntrinsic(value string, parameters map[string]string) string {
	if value == "" || len(parameters) == 0 {
		return value
	}
	return subPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := subPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		if replacement, ok := parameters[parts[1]]; ok {
			return replacement
		}
		return match
	})
}

func ensureTrailingSlash(value string) string {
	if value == "" {
		return value
	}
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func asInt(value any) int {
	if val, ok := asIntPointer(value); ok {
		return *val
	}
	return 0
}

func asIntDefault(value any, fallback int) int {
	if val, ok := asIntPointer(value); ok {
		return *val
	}
	return fallback
}
