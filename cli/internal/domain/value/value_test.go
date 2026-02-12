// Where: cli/internal/domain/value/value_test.go
// What: Tests for value conversion helpers.
// Why: Keep parsing helpers stable across refactors.
package value

import (
	"reflect"
	"testing"
)

func TestValueHelpers(t *testing.T) {
	if got := AsString("hello"); got != "hello" {
		t.Errorf("AsString(hello) = %s", got)
	}
	if got := AsString(123); got != "123" {
		t.Errorf("AsString(123) = %s", got)
	}
	if got := AsString(true); got != "true" {
		t.Errorf("AsString(true) = %s", got)
	}

	if got := AsInt("123"); got != 123 {
		t.Errorf("AsInt(123) = %d", got)
	}
	if got := AsInt("invalid"); got != 0 {
		t.Errorf("AsInt(invalid) = %d", got)
	}

	ptr, ok := AsIntPointer("123")
	if !ok || ptr == nil || *ptr != 123 {
		t.Errorf("AsIntPointer(123) = %v, %v", ptr, ok)
	}

	slice := AsSlice([]any{"a", "b"})
	if !reflect.DeepEqual(slice, []any{"a", "b"}) {
		t.Errorf("AsSlice = %v", slice)
	}
	slice = AsSlice("scalar")
	if !reflect.DeepEqual(slice, []any{"scalar"}) {
		t.Errorf("AsSlice(scalar) = %v", slice)
	}

	m := AsMap(map[string]any{"a": 1})
	if m["a"] != 1 {
		t.Errorf("AsMap = %v", m)
	}
	if AsMap("not a map") != nil {
		t.Errorf("AsMap(scalar) should be nil")
	}

	if got := EnsureTrailingSlash("path"); got != "path/" {
		t.Errorf("EnsureTrailingSlash(path) = %s", got)
	}
	if got := EnsureTrailingSlash("path/"); got != "path/" {
		t.Errorf("EnsureTrailingSlash(path/) = %s", got)
	}

	envMap := EnvSliceToMap([]string{
		"A=1",
		" B =2",
		"NO_EQUALS",
		"",
		"=skip",
		"A=override",
	})
	if envMap["A"] != "override" {
		t.Errorf("EnvSliceToMap A = %s", envMap["A"])
	}
	if envMap["B"] != "2" {
		t.Errorf("EnvSliceToMap B = %s", envMap["B"])
	}
	if envMap["NO_EQUALS"] != "" {
		t.Errorf("EnvSliceToMap NO_EQUALS = %s", envMap["NO_EQUALS"])
	}
	if _, ok := envMap[""]; ok {
		t.Errorf("EnvSliceToMap should ignore empty key")
	}
}
