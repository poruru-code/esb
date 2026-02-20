// Where: cli/internal/infra/sam/intrinsics_resolve_dispatch.go
// What: Intrinsic function dispatch for resolver.Resolve.
// Why: Keep the large intrinsic-switching logic isolated from resolver state helpers.
package sam

import (
	"fmt"
	"strings"

	"github.com/poruru-code/esb/cli/internal/domain/value"
)

// Resolve implements parser.Resolver.
func (r *IntrinsicResolver) Resolve(ctx *Context, node any) (any, bool, error) {
	if node == nil {
		return nil, false, nil
	}

	if s, ok := node.(string); ok {
		resolved := resolveIntrinsicWithParams(r.Parameters, s)
		if resolved != s {
			return resolved, true, nil
		}
		return node, false, nil
	}

	m, ok := node.(map[string]any)
	if !ok {
		return node, false, nil
	}

	if ref, ok := m["Ref"]; ok && len(m) == 1 {
		resolvedRef := r.resolveValue(ctx, ref)
		if s, ok := resolvedRef.(string); ok {
			if val, ok := r.Parameters[s]; ok {
				return val, true, nil
			}
			if strings.HasPrefix(s, "AWS::") {
				return "local-" + s[5:], true, nil
			}
			return s, true, nil
		}
		return node, false, nil
	}

	if ifVal, ok := m["Fn::If"]; ok && len(m) == 1 {
		if args, ok := ifVal.([]any); ok && len(args) == 3 {
			condName := value.AsString(args[0])
			if r.GetConditionResult(condName) {
				return args[1], true, nil
			}
			return args[2], true, nil
		}
		r.addWarning("Fn::If: arguments must be [condition, true_val, false_val]")
		return node, false, nil
	}

	if sub, ok := m["Fn::Sub"]; ok && len(m) == 1 {
		switch typed := sub.(type) {
		case string:
			return resolveIntrinsicWithParams(r.Parameters, typed), true, nil
		case []any:
			if len(typed) == 2 {
				template := value.AsString(typed[0])
				vars, isMap := typed[1].(map[string]any)
				if template == "" {
					r.addWarning("Fn::Sub: template string is empty")
					return node, false, nil
				}
				params := map[string]string{}
				for k, v := range r.Parameters {
					params[k] = v
				}
				if isMap {
					for k, v := range vars {
						params[k] = value.AsString(r.resolveValue(ctx, v))
					}
				}
				return resolveIntrinsicWithParams(params, template), true, nil
			}
		}
	}

	if join, ok := m["Fn::Join"]; ok && len(m) == 1 {
		if args, ok := join.([]any); ok && len(args) == 2 {
			sep := value.AsString(r.resolveValue(ctx, args[0]))
			elements, isSlice := args[1].([]any)
			if !isSlice {
				return node, false, nil
			}
			resolvedElements := make([]string, 0, len(elements))
			for _, el := range elements {
				resolvedElements = append(resolvedElements, value.AsString(r.resolveValue(ctx, el)))
			}
			return strings.Join(resolvedElements, sep), true, nil
		}
		r.addWarning("Fn::Join: arguments must be [sep, [elements]]")
		return node, false, nil
	}

	if getAtt, ok := m["Fn::GetAtt"]; ok && len(m) == 1 {
		var resName, attrName string
		switch typed := getAtt.(type) {
		case string:
			parts := strings.Split(typed, ".")
			if len(parts) == 2 {
				resName = parts[0]
				attrName = parts[1]
			} else {
				r.addWarningf("Fn::GetAtt: malformed string %q", typed)
				return node, false, nil
			}
		case []any:
			if len(typed) == 2 {
				resName = value.AsString(r.resolveValue(ctx, typed[0]))
				attrName = value.AsString(r.resolveValue(ctx, typed[1]))
			} else {
				r.addWarning("Fn::GetAtt: array must have 2 elements")
				return node, false, nil
			}
		default:
			r.addWarningf("Fn::GetAtt: unsupported type %T", typed)
			return node, false, nil
		}
		return fmt.Sprintf("arn:aws:local:%s:global:%s/%s", attrName, resName, attrName), true, nil
	}

	if split, ok := m["Fn::Split"]; ok && len(m) == 1 {
		if args, ok := split.([]any); ok && len(args) == 2 {
			delimiter := value.AsString(r.resolveValue(ctx, args[0]))
			source := value.AsString(r.resolveValue(ctx, args[1]))
			parts := strings.Split(source, delimiter)
			out := make([]any, len(parts))
			for i, p := range parts {
				out[i] = p
			}
			return out, true, nil
		}
	}

	if selectInt, ok := m["Fn::Select"]; ok && len(m) == 1 {
		if args, ok := selectInt.([]any); ok && len(args) == 2 {
			index := value.AsInt(r.resolveValue(ctx, args[0]))
			list := value.AsSlice(r.resolveValue(ctx, args[1]))
			if index >= 0 && index < len(list) {
				return list[index], true, nil
			}
		}
	}

	if importVal, ok := m["Fn::ImportValue"]; ok && len(m) == 1 {
		name := value.AsString(r.resolveValue(ctx, importVal))
		return "imported-" + name, true, nil
	}

	return node, false, nil
}
