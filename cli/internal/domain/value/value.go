// Where: cli/internal/domain/value/value.go
// What: Value conversion helpers for parsed template data.
// Why: Keep parsing/merge logic concise without infrastructure dependencies.
package value

import (
	"fmt"
	"strconv"
	"strings"
)

// AsMap converts a value to map form when possible.
func AsMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

// AsSlice converts a value to slice form, wrapping scalars when needed.
func AsSlice(value any) []any {
	if value == nil {
		return nil
	}
	if v, ok := value.([]any); ok {
		return v
	}
	return []any{value}
}

// AsString returns the string representation of a value.
func AsString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// AsStringDefault returns a string representation or the fallback.
func AsStringDefault(value any, fallback string) string {
	if out := AsString(value); out != "" {
		return out
	}
	return fallback
}

// AsIntPointer attempts to coerce a value into an int pointer.
func AsIntPointer(value any) (*int, bool) {
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

// AsInt converts a value to int, returning 0 when conversion fails.
func AsInt(value any) int {
	if val, ok := AsIntPointer(value); ok {
		return *val
	}
	return 0
}

// AsIntDefault converts a value to int or returns the fallback.
func AsIntDefault(value any, fallback int) int {
	if val, ok := AsIntPointer(value); ok {
		return *val
	}
	return fallback
}

// EnsureTrailingSlash appends a trailing slash when missing.
func EnsureTrailingSlash(value string) string {
	if value == "" {
		return value
	}
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

// EnvSliceToMap converts container-style env entries (KEY=VALUE) into a map.
func EnvSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		out[key] = value
	}
	return out
}
