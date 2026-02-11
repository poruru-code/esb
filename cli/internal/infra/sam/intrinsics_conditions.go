// Where: cli/internal/infra/sam/intrinsics_conditions.go
// What: Condition cache and evaluator for intrinsic resolver.
// Why: Separate conditional expression handling from Resolve dispatch logic.
package sam

import (
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

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
