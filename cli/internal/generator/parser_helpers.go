package generator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParserContext encapsulates parameters and provides intrinsic function resolution.
type ParserContext struct {
	Parameters map[string]string
}

func NewParserContext(params map[string]string) *ParserContext {
	if params == nil {
		params = make(map[string]string)
	}
	return &ParserContext{Parameters: params}
}

func (ctx *ParserContext) mapToStruct(input, output any) error {
	resolved := ctx.resolveRecursively(input, 0)
	data, err := json.Marshal(resolved)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}

const maxResolveDepth = 20

func (ctx *ParserContext) resolveRecursively(val any, depth int) any {
	if val == nil || depth > maxResolveDepth {
		return val
	}

	switch v := val.(type) {
	case map[string]any:
		// First, resolve any intrinsics at this level
		resolvedMap := ctx.resolve(v)
		// If the resolution resulted in a scalar or slice, return it directly
		if _, isMap := resolvedMap.(map[string]any); !isMap {
			return resolvedMap
		}
		// Otherwise, it's still a map, so recurse into its values
		rMap := resolvedMap.(map[string]any)
		newMap := make(map[string]any, len(rMap))
		for k, sv := range rMap {
			newMap[k] = ctx.resolveRecursively(sv, depth+1)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(v))
		for i, sv := range v {
			newSlice[i] = ctx.resolveRecursively(sv, depth+1)
		}
		return newSlice
	default:
		// For scalar values, just resolve them (e.g., string with ${param})
		return ctx.resolve(v)
	}
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func (ctx *ParserContext) asMap(value any) map[string]any {
	resolved := ctx.resolve(value)
	if m, ok := resolved.(map[string]any); ok {
		return m
	}
	return nil
}

func (ctx *ParserContext) asSlice(value any) []any {
	resolved := ctx.resolve(value)
	if resolved == nil {
		return nil
	}
	if v, ok := resolved.([]any); ok {
		return v
	}
	// Handle single element as slice
	return []any{resolved}
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

func (ctx *ParserContext) asString(value any) string {
	resolved := ctx.resolve(value)
	switch typed := resolved.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		if resolved == nil {
			return ""
		}
		return fmt.Sprint(resolved)
	}
}

func (ctx *ParserContext) asStringDefault(value any, fallback string) string {
	if out := ctx.asString(value); out != "" {
		return out
	}
	return fallback
}

func (ctx *ParserContext) asIntPointer(value any) (*int, bool) {
	resolved := ctx.resolve(value)
	switch typed := resolved.(type) {
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

var subPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_:]+)\}`)

func (ctx *ParserContext) resolveIntrinsic(value string) string {
	if value == "" || len(ctx.Parameters) == 0 {
		return value
	}
	return subPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := subPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		paramName := parts[1]
		if replacement, ok := ctx.Parameters[paramName]; ok {
			// Parameters are always stored as strings, so no need to call asString here.
			return replacement
		}
		// Fallback for pseudo parameters
		if strings.HasPrefix(paramName, "AWS::") {
			return "local-" + paramName[5:]
		}
		return match
	})
}

func (ctx *ParserContext) resolve(value any) any {
	if value == nil {
		return nil
	}

	m, ok := value.(map[string]any)
	if !ok {
		if s, ok := value.(string); ok {
			return ctx.resolveIntrinsic(s)
		}
		return value
	}

	// !Ref
	if ref, ok := m["Ref"]; ok && len(m) == 1 {
		// Resolve the Ref value itself first, in case it's an intrinsic
		resolvedRef := ctx.resolve(ref)
		if s, ok := resolvedRef.(string); ok {
			if val, ok := ctx.Parameters[s]; ok {
				return val
			}
			// Fallback for pseudo parameters
			if strings.HasPrefix(s, "AWS::") {
				return "local-" + s[5:]
			}
			// Fallback for resource references: AWS Ref to a resource returns its name
			return s
		}
		return value // If resolvedRef is not a string, leave as is
	}

	// !Sub
	if sub, ok := m["Fn::Sub"]; ok && len(m) == 1 {
		switch typed := sub.(type) {
		case string:
			return ctx.resolveIntrinsic(typed)
		case []any:
			// List version: [template, {vars}]
			if len(typed) == 2 {
				template := ctx.asString(typed[0]) // Resolve template string
				vars, isMap := typed[1].(map[string]any)
				if template == "" {
					return value
				}
				// Create temporary context with merged parameters
				tempParams := make(map[string]string)
				for k, v := range ctx.Parameters {
					tempParams[k] = v
				}
				if isMap {
					for k, v := range vars {
						// Recursively resolve values in variables map
						tempParams[k] = ctx.asString(ctx.resolveRecursively(v, 0))
					}
				}
				tempCtx := &ParserContext{Parameters: tempParams}
				return tempCtx.resolveIntrinsic(template)
			}
		}
	}

	// !Join
	if join, ok := m["Fn::Join"]; ok && len(m) == 1 {
		if args, ok := join.([]any); ok && len(args) == 2 {
			sep := ctx.asString(args[0]) // Resolve separator
			elements, isSlice := args[1].([]any)
			if !isSlice {
				return value
			}
			resolvedElements := make([]string, 0, len(elements))
			for _, el := range elements {
				resolvedElements = append(resolvedElements, ctx.asString(ctx.resolveRecursively(el, 0)))
			}
			return strings.Join(resolvedElements, sep)
		}
	}

	// !GetAtt
	if getAtt, ok := m["Fn::GetAtt"]; ok && len(m) == 1 {
		var resName, attrName string
		switch typed := getAtt.(type) {
		case string:
			// !GetAtt Resource.Attribute
			parts := strings.Split(typed, ".")
			if len(parts) == 2 {
				resName = parts[0]
				attrName = parts[1]
			} else {
				return value // Malformed GetAtt string
			}
		case []any:
			// !GetAtt [Resource, Attribute]
			if len(typed) == 2 {
				resName = ctx.asString(ctx.resolveRecursively(typed[0], 0))
				attrName = ctx.asString(ctx.resolveRecursively(typed[1], 0))
			} else {
				return value // Malformed GetAtt array
			}
		default:
			return value // Unsupported type for GetAtt
		}
		// We can't fully resolve dynamically allocated ARNs here,
		// but we can provide a deterministic placeholder.
		return fmt.Sprintf("arn:aws:local:%s:global:%s/%s", attrName, resName, attrName)
	}

	// !Split
	if split, ok := m["Fn::Split"]; ok && len(m) == 1 {
		if args, ok := split.([]any); ok && len(args) == 2 {
			delimiter := ctx.asString(args[0])
			source := ctx.asString(ctx.resolveRecursively(args[1], 0))
			parts := strings.Split(source, delimiter)
			out := make([]any, len(parts))
			for i, p := range parts {
				out[i] = p
			}
			return out
		}
	}

	// !Select
	if selectInt, ok := m["Fn::Select"]; ok && len(m) == 1 {
		if args, ok := selectInt.([]any); ok && len(args) == 2 {
			index := ctx.asInt(args[0])
			list := ctx.asSlice(ctx.resolveRecursively(args[1], 0))
			if index >= 0 && index < len(list) {
				return list[index]
			}
		}
	}

	// !ImportValue
	if importVal, ok := m["Fn::ImportValue"]; ok && len(m) == 1 {
		name := ctx.asString(ctx.resolveRecursively(importVal, 0))
		return "imported-" + name
	}

	return value
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

func (ctx *ParserContext) asInt(value any) int {
	if val, ok := ctx.asIntPointer(value); ok {
		return *val
	}
	return 0
}

func (ctx *ParserContext) asIntDefault(value any, fallback int) int {
	if val, ok := ctx.asIntPointer(value); ok {
		return *val
	}
	return fallback
}

// (Removed redundant top-level mapToStruct)
