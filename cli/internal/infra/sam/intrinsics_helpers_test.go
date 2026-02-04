// Where: cli/internal/infra/sam/intrinsics_helpers_test.go
// What: Tests for intrinsic resolver and value env.
// Why: Keep intrinsic handling stable for ESB parsing.
package sam

import (
	"reflect"
	"testing"
)

func TestIntrinsicResolver_Conditions_Advanced(t *testing.T) {
	resolver := NewIntrinsicResolver(map[string]string{
		"Env": "prod",
	})
	resolver.RawConditions = map[string]any{
		"IsProd":   true,
		"IsDev":    false,
		"IsTrue":   "true",
		"IsOne":    "1",
		"IsFalse":  "False",
		"Nested":   map[string]any{"Condition": "IsProd"},
		"AndTrue":  map[string]any{"Fn::And": []any{map[string]any{"Condition": "IsProd"}, true}},
		"AndFalse": map[string]any{"Fn::And": []any{map[string]any{"Condition": "IsProd"}, false}},
		"OrTrue":   map[string]any{"Fn::Or": []any{map[string]any{"Condition": "IsDev"}, true}},
		"OrFalse":  map[string]any{"Fn::Or": []any{map[string]any{"Condition": "IsDev"}, false}},
		"NotTrue":  map[string]any{"Fn::Not": []any{map[string]any{"Condition": "IsDev"}}},
		"NotFalse": map[string]any{"Fn::Not": []any{map[string]any{"Condition": "IsProd"}}},
		"CircA":    map[string]any{"Condition": "CircB"},
		"CircB":    map[string]any{"Condition": "CircA"},
	}

	tests := []struct {
		name string
		cond string
		want bool
	}{
		{"bare true", "IsProd", true},
		{"bare false", "IsDev", false},
		{"string true", "IsTrue", true},
		{"string 1", "IsOne", true},
		{"string false", "IsFalse", false},
		{"nested", "Nested", true},
		{"and true", "AndTrue", true},
		{"and false", "AndFalse", false},
		{"or true", "OrTrue", true},
		{"or false", "OrFalse", false},
		{"not true", "NotTrue", true},
		{"not false", "NotFalse", false},
		{"circular", "CircA", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.GetConditionResult(tt.cond)
			if got != tt.want {
				t.Errorf("GetConditionResult(%s) = %v, want %v", tt.cond, got, tt.want)
			}
		})
	}

	found := false
	for _, w := range resolver.Warnings {
		if w == "Circular dependency detected in condition \"CircA\"" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected circular dependency warning, got: %v", resolver.Warnings)
	}
}

func TestIntrinsicResolver_Warnings(t *testing.T) {
	resolver := NewIntrinsicResolver(nil)
	resolver.addWarning("test warning")
	resolver.addWarning("test warning")
	resolver.addWarningf("formatted %s", "warning")

	expected := []string{"test warning", "formatted warning"}
	if !reflect.DeepEqual(resolver.Warnings, expected) {
		t.Errorf("Warnings = %v, want %v", resolver.Warnings, expected)
	}
}

func TestIntrinsicResolver_IntrinsicsErrors(t *testing.T) {
	resolver := NewIntrinsicResolver(nil)
	ctx := &Context{MaxDepth: maxResolveDepth}

	input := map[string]any{"Fn::Join": "invalid"}
	resolved, err := ResolveAll(ctx, input, resolver)
	if err != nil {
		t.Fatalf("ResolveAll error: %v", err)
	}
	if !reflect.DeepEqual(resolved, input) {
		t.Errorf("ResolveAll = %v, want %v", resolved, input)
	}

	found := false
	for _, w := range resolver.Warnings {
		if w == "Fn::Join: arguments must be [sep, [elements]]" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected Fn::Join warning")
	}
}
