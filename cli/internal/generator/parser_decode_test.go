package generator

import (
	"reflect"
	"testing"
)

func TestDecodeYAML_Advanced(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]any
	}{
		{
			name: "scalar tags",
			content: `
A: !Ref MyParam
B: !Sub "hello-${Name}"
C: !GetAtt Res.Attr
D: !ImportValue ExportName
E: !Condition MyCond
`,
			want: map[string]any{
				"A": map[string]any{"Ref": "MyParam"},
				"B": map[string]any{"Fn::Sub": "hello-${Name}"},
				"C": map[string]any{"Fn::GetAtt": "Res.Attr"},
				"D": map[string]any{"Fn::ImportValue": "ExportName"},
				"E": map[string]any{"Condition": "MyCond"},
			},
		},
		{
			name: "sequence tags",
			content: `
A: !Join [ ":", [ a, b ] ]
B: !Equals [ a, b ]
C: !And [ true, false ]
D: !Or [ true, false ]
E: !Not [ true ]
F: !Select [ 0, [ a, b ] ]
G: !Split [ ":", "a:b" ]
`,
			want: map[string]any{
				"A": map[string]any{"Fn::Join": []any{":", []any{"a", "b"}}},
				"B": map[string]any{"Fn::Equals": []any{"a", "b"}},
				"C": map[string]any{"Fn::And": []any{true, false}},
				"D": map[string]any{"Fn::Or": []any{true, false}},
				"E": map[string]any{"Fn::Not": []any{true}},
				"F": map[string]any{"Fn::Select": []any{0, []any{"a", "b"}}},
				"G": map[string]any{"Fn::Split": []any{":", "a:b"}},
			},
		},
		{
			name: "mapping tags",
			content: `
A: !Sub
  Name: world
  Template: hello-${Name}
`,
			want: map[string]any{
				"A": map[string]any{
					"Fn::Sub": map[string]any{
						"Name":     "world",
						"Template": "hello-${Name}",
					},
				},
			},
		},
		{
			name: "basic types",
			content: `
Int: 123
Float: 1.23
Bool: true
N: null
`,
			want: map[string]any{
				"Int":   123,
				"Float": 1.23,
				"Bool":  true,
				"N":     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeYAML(tt.content)
			if err != nil {
				t.Fatalf("decodeYAML() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeYAML_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"invalid yaml", "[:"},
		{"empty yaml", ""},
		{"sequence root", "- a\n- b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeYAML(tt.content)
			if err == nil {
				t.Errorf("decodeYAML() expected error for %s", tt.name)
			}
		})
	}
}
