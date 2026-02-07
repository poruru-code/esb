// Where: cli/internal/command/deploy_image_prewarm_test.go
// What: Unit tests for deploy --image-prewarm flag normalization.
// Why: Keep CLI validation behavior deterministic.
package command

import "testing"

func TestNormalizeImagePrewarm(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: "all"},
		{input: "off", want: "off"},
		{input: "all", want: "all"},
		{input: "ALL", want: "all"},
		{input: "invalid", wantErr: true},
	}
	for _, tc := range cases {
		got, err := normalizeImagePrewarm(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for input %q", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for input %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("unexpected value for input %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}
