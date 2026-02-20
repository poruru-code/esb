package yamlshape

import "fmt"

// AsMap converts YAML-decoded map values into map[string]any.
// It accepts map[string]any and map[any]any and skips non-string keys.
func AsMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			name, ok := key.(string)
			if !ok {
				continue
			}
			out[name] = val
		}
		return out
	default:
		return map[string]any{}
	}
}

// AsSlice converts YAML-decoded sequence values into []any.
func AsSlice(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

// RouteKey returns a stable route key in "<path>:<method>" format.
// Missing or empty method defaults to GET.
func RouteKey(route map[string]any) string {
	path, _ := route["path"].(string)
	method, _ := route["method"].(string)
	if path == "" {
		return ""
	}
	if method == "" {
		method = "GET"
	}
	return fmt.Sprintf("%s:%s", path, method)
}
