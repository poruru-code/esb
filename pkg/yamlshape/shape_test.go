package yamlshape

import "testing"

func TestAsMap(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  map[string]any
	}{
		{
			name:  "map string any",
			input: map[string]any{"name": "value"},
			want:  map[string]any{"name": "value"},
		},
		{
			name:  "yaml map any any",
			input: map[any]any{"a": 1, "b": "x", 123: "ignored"},
			want:  map[string]any{"a": 1, "b": "x"},
		},
		{
			name:  "nil value",
			input: nil,
			want:  map[string]any{},
		},
		{
			name:  "invalid type",
			input: "not a map",
			want:  map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AsMap(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("map length mismatch: got=%d want=%d", len(got), len(tc.want))
			}
			for key, wantValue := range tc.want {
				gotValue, ok := got[key]
				if !ok {
					t.Fatalf("missing key: %s", key)
				}
				if gotValue != wantValue {
					t.Fatalf("value mismatch for %s: got=%v want=%v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestAsSlice(t *testing.T) {
	values := []any{"a", 1}
	got := AsSlice(values)
	if len(got) != 2 || got[0] != "a" || got[1] != 1 {
		t.Fatalf("unexpected slice: %#v", got)
	}

	if got := AsSlice("scalar"); got != nil {
		t.Fatalf("expected nil for scalar, got %#v", got)
	}

	if got := AsSlice(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %#v", got)
	}
}

func TestRouteKey(t *testing.T) {
	tests := []struct {
		name  string
		route map[string]any
		want  string
	}{
		{
			name:  "path and method",
			route: map[string]any{"path": "/v1/ping", "method": "POST"},
			want:  "/v1/ping:POST",
		},
		{
			name:  "default method",
			route: map[string]any{"path": "/v1/ping"},
			want:  "/v1/ping:GET",
		},
		{
			name:  "missing path",
			route: map[string]any{"method": "GET"},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteKey(tt.route)
			if got != tt.want {
				t.Fatalf("RouteKey(%#v) = %q, want %q", tt.route, got, tt.want)
			}
		})
	}
}
