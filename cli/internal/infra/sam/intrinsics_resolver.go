// Where: cli/internal/infra/sam/intrinsics_resolver.go
// What: Intrinsic resolver for SAM templates.
// Why: Resolve parameters and intrinsic functions before parsing resources.
package sam

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

const maxResolveDepth = 20

// IntrinsicResolver resolves CloudFormation/SAM intrinsic functions.
type IntrinsicResolver struct {
	Parameters     map[string]string
	RawConditions  map[string]any
	ConditionCache map[string]bool
	ConditionStack map[string]bool
	Warnings       []string
	warningsSeen   map[string]struct{}
}

// NewIntrinsicResolver builds a resolver with parameter values.
func NewIntrinsicResolver(params map[string]string) *IntrinsicResolver {
	if params == nil {
		params = map[string]string{}
	}
	return &IntrinsicResolver{
		Parameters:     params,
		RawConditions:  map[string]any{},
		ConditionCache: map[string]bool{},
		ConditionStack: map[string]bool{},
		warningsSeen:   map[string]struct{}{},
	}
}

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

	// !Ref
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

	// !If
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

	// !Sub
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

	// !Join
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

	// !GetAtt
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

	// !Split
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

	// !Select
	if selectInt, ok := m["Fn::Select"]; ok && len(m) == 1 {
		if args, ok := selectInt.([]any); ok && len(args) == 2 {
			index := value.AsInt(r.resolveValue(ctx, args[0]))
			list := value.AsSlice(r.resolveValue(ctx, args[1]))
			if index >= 0 && index < len(list) {
				return list[index], true, nil
			}
		}
	}

	// !ImportValue
	if importVal, ok := m["Fn::ImportValue"]; ok && len(m) == 1 {
		name := value.AsString(r.resolveValue(ctx, importVal))
		return "imported-" + name, true, nil
	}

	return node, false, nil
}

func (r *IntrinsicResolver) resolveValue(ctx *Context, node any) any {
	if ctx == nil {
		ctx = &Context{MaxDepth: maxResolveDepth}
	}
	resolved, err := ResolveAll(ctx, node, r)
	if err != nil {
		r.addWarningf("resolve error: %v", err)
		return node
	}
	return resolved
}

var subPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_:]+)\}`)

func resolveIntrinsicWithParams(params map[string]string, value string) string {
	if value == "" || len(params) == 0 {
		return value
	}
	return subPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := subPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		paramName := parts[1]
		if replacement, ok := params[paramName]; ok {
			return replacement
		}
		if strings.HasPrefix(paramName, "AWS::") {
			return "local-" + paramName[5:]
		}
		return match
	})
}

func (r *IntrinsicResolver) GetConditionResult(name string) bool {
	if res, ok := r.ConditionCache[name]; ok {
		return res
	}
	raw, ok := r.RawConditions[name]
	if !ok {
		r.addWarningf("Condition %q not found", name)
		return false
	}

	if r.ConditionStack[name] {
		r.addWarningf("Circular dependency detected in condition %q", name)
		return false
	}

	r.ConditionStack[name] = true
	defer delete(r.ConditionStack, name)

	res := r.EvaluateCondition(raw)
	r.ConditionCache[name] = res
	return res
}

func (r *IntrinsicResolver) EvaluateCondition(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		switch typed := node.(type) {
		case bool:
			return typed
		case string:
			return typed == "true" || typed == "True" || typed == "1"
		default:
			resolved := r.resolveValue(nil, node)
			if b, ok := resolved.(bool); ok {
				return b
			}
			if s, ok := resolved.(string); ok {
				return s == "true" || s == "True" || s == "1"
			}
			return false
		}
	}

	if eq, ok := m["Fn::Equals"]; ok {
		if args, ok := eq.([]any); ok && len(args) == 2 {
			v1 := r.resolveValue(nil, args[0])
			v2 := r.resolveValue(nil, args[1])
			return fmt.Sprint(v1) == fmt.Sprint(v2)
		}
	}

	if not, ok := m["Fn::Not"]; ok {
		if args, ok := not.([]any); ok && len(args) == 1 {
			return !r.EvaluateCondition(args[0])
		}
	}

	if and, ok := m["Fn::And"]; ok {
		if args, ok := and.([]any); ok {
			for _, arg := range args {
				if !r.EvaluateCondition(arg) {
					return false
				}
			}
			return true
		}
	}

	if or, ok := m["Fn::Or"]; ok {
		if args, ok := or.([]any); ok {
			for _, arg := range args {
				if r.EvaluateCondition(arg) {
					return true
				}
			}
			return false
		}
	}

	if cond, ok := m["Condition"]; ok {
		return r.GetConditionResult(value.AsString(cond))
	}

	return false
}

func (r *IntrinsicResolver) addWarning(msg string) {
	if _, seen := r.warningsSeen[msg]; seen {
		return
	}
	r.Warnings = append(r.Warnings, msg)
	r.warningsSeen[msg] = struct{}{}
}

func (r *IntrinsicResolver) addWarningf(format string, args ...any) {
	r.addWarning(fmt.Sprintf(format, args...))
}
