// Where: cli/internal/generator/value_helpers.go
// What: Value conversion helpers for parsed SAM data.
// Why: Keep parsing code concise and consistent.
package generator

import (
	"fmt"
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
	return []any{value}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
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

func ensureTrailingSlash(value string) string {
	if value == "" {
		return value
	}
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
