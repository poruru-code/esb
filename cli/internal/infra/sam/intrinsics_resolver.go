// Where: cli/internal/infra/sam/intrinsics_resolver.go
// What: Core resolver state and shared helpers for SAM intrinsics.
// Why: Keep resolver contracts compact while dispatch/condition logic lives in dedicated files.
package sam

import (
	"regexp"
	"strings"
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
