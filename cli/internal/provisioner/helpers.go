// Where: cli/internal/provisioner/helpers.go
// What: Small conversion helpers for provisioner inputs.
// Why: Normalize loosely-typed YAML data into typed values.
package provisioner

import (
	"fmt"
	"strconv"
)

func toString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func toStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := toString(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toInt64(value any) (int64, error) {
	if value == nil {
		return 0, fmt.Errorf("value is nil")
	}
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case uint:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case string:
		if v == "" {
			return 0, fmt.Errorf("value is empty string")
		}
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, nil
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f), nil
		}
		return 0, fmt.Errorf("failed to parse string '%s' as integer", v)
	default:
		return 0, fmt.Errorf("unsupported type %T for integer conversion", value)
	}
}
