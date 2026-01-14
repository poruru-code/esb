package generator

import (
	"reflect"
	"testing"
)

func TestParserContext_Conditions_Advanced(t *testing.T) {
	ctx := NewParserContext(map[string]string{
		"Env": "prod",
	})
	ctx.RawConditions = map[string]any{
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
		// Circular
		"CircA": map[string]any{"Condition": "CircB"},
		"CircB": map[string]any{"Condition": "CircA"},
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
		{"circular", "CircA", false}, // Should return false and add warning
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.GetConditionResult(tt.cond)
			if got != tt.want {
				t.Errorf("GetConditionResult(%s) = %v, want %v", tt.cond, got, tt.want)
			}
		})
	}

	// Check warnings for circularity
	found := false
	for _, w := range ctx.Warnings {
		if w == "Circular dependency detected in condition \"CircA\"" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected circular dependency warning, got: %v", ctx.Warnings)
	}
}

func TestParserContext_Warnings(t *testing.T) {
	ctx := NewParserContext(nil)
	ctx.addWarning("test warning")
	ctx.addWarning("test warning") // Should be deduplicated
	ctx.addWarningf("formatted %s", "warning")

	expected := []string{"test warning", "formatted warning"}
	if !reflect.DeepEqual(ctx.Warnings, expected) {
		t.Errorf("Warnings = %v, want %v", ctx.Warnings, expected)
	}
}

func TestParserContext_Intrinsics_Errors(t *testing.T) {
	ctx := NewParserContext(nil)

	// Malformed Fn::Join
	input := map[string]any{"Fn::Join": "invalid"}
	res := ctx.resolve(input, 0)
	if !reflect.DeepEqual(res, input) {
		t.Errorf("Fn::Join with invalid args should return input itself, got %v", res)
	}

	found := false
	for _, w := range ctx.Warnings {
		if w == "Fn::Join: arguments must be [sep, [elements]]" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected Fn::Join warning")
	}

	// Max depth
	highDepth := map[string]any{"Ref": "A"}
	for i := 0; i < 25; i++ {
		highDepth = map[string]any{"Fn::If": []any{"IsProd", highDepth, highDepth}}
	}
	ctx.resolveRecursively(highDepth, 0)

	found = false
	for _, w := range ctx.Warnings {
		if w == "Max resolve depth reached, returning raw value" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected Max resolve depth warning, got: %v", ctx.Warnings)
	}
}

func TestParserContext_Utilities(t *testing.T) {
	ctx := NewParserContext(map[string]string{"Count": "10", "Price": "1.5"})

	// asString
	if got := ctx.asString("hello"); got != "hello" {
		t.Errorf("asString(hello) = %s", got)
	}
	if got := ctx.asString(123); got != "123" {
		t.Errorf("asString(123) = %s", got)
	}
	if got := ctx.asString(true); got != "true" {
		t.Errorf("asString(true) = %s", got)
	}
	if got := ctx.asString(map[string]any{"Ref": "Count"}); got != "10" {
		t.Errorf("asString(!Ref Count) = %s", got)
	}

	// asInt
	if got := ctx.asInt("123"); got != 123 {
		t.Errorf("asInt(123) = %d", got)
	}
	if got := ctx.asInt(map[string]any{"Ref": "Count"}); got != 10 {
		t.Errorf("asInt(!Ref Count) = %d", got)
	}
	if got := ctx.asInt("invalid"); got != 0 {
		t.Errorf("asInt(invalid) = %d", got)
	}

	// asIntPointer
	ptr, ok := ctx.asIntPointer("123")
	if !ok || ptr == nil || *ptr != 123 {
		t.Errorf("asIntPointer(123) = %v, %v", ptr, ok)
	}
	ptr, ok = ctx.asIntPointer("invalid")
	if ok || ptr != nil {
		t.Errorf("asIntPointer(invalid) should be false/nil, got %v, %v", ptr, ok)
	}
	i64 := int64(456)
	ptr, ok = ctx.asIntPointer(i64)
	if !ok || *ptr != 456 {
		t.Errorf("asIntPointer(int64) = %v, %v", ptr, ok)
	}
	f64 := 7.89
	ptr, ok = ctx.asIntPointer(f64)
	if !ok || *ptr != 7 {
		t.Errorf("asIntPointer(float64) = %v, %v", ptr, ok)
	}

	// asSlice
	slice := ctx.asSlice([]any{"a", "b"})
	if !reflect.DeepEqual(slice, []any{"a", "b"}) {
		t.Errorf("asSlice = %v", slice)
	}
	// Feature: Handle single element as slice
	slice = ctx.asSlice("scalar")
	if !reflect.DeepEqual(slice, []any{"scalar"}) {
		t.Errorf("asSlice(scalar) = %v, want [scalar]", slice)
	}

	// asMap
	m := ctx.asMap(map[string]any{"a": 1})
	if m["a"] != 1 {
		t.Errorf("asMap = %v", m)
	}
	if ctx.asMap("not a map") != nil {
		t.Errorf("asMap(scalar) should be nil")
	}

	// asString (fmt.Stringer)
	if got := ctx.asString(dummyStringer{}); got != "dummy" {
		t.Errorf("asString(Stringer) = %s", got)
	}

	// ensureTrailingSlash
	if got := ensureTrailingSlash("path"); got != "path/" {
		t.Errorf("ensureTrailingSlash(path) = %s", got)
	}
	if got := ensureTrailingSlash("path/"); got != "path/" {
		t.Errorf("ensureTrailingSlash(path/) = %s", got)
	}
}

type dummyStringer struct{}

func (d dummyStringer) String() string { return "dummy" }
