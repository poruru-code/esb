package engine

func asMap(value any) map[string]any {
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

func asSlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}
