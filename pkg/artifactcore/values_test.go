package artifactcore

import "testing"

func TestAsMapHandlesSupportedAndFallbackTypes(t *testing.T) {
	direct := map[string]any{"k": "v"}
	if got := asMap(direct); got["k"] != "v" {
		t.Fatalf("asMap(map[string]any) = %#v", got)
	}

	mixed := map[any]any{
		"name": "alice",
		123:    "ignored",
	}
	converted := asMap(mixed)
	if converted["name"] != "alice" {
		t.Fatalf("asMap(map[any]any) = %#v", converted)
	}
	if _, ok := converted["123"]; ok {
		t.Fatalf("expected non-string key to be skipped: %#v", converted)
	}

	empty := asMap("not-map")
	if len(empty) != 0 {
		t.Fatalf("expected fallback empty map, got %#v", empty)
	}
}

func TestAsSliceHandlesSupportedAndFallbackTypes(t *testing.T) {
	input := []any{"a", "b"}
	out := asSlice(input)
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("asSlice([]any) = %#v", out)
	}

	if got := asSlice("not-slice"); got != nil {
		t.Fatalf("expected nil fallback, got %#v", got)
	}
}
