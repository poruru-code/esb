// Where: cli/internal/infra/interaction/interaction_test.go
// What: Tests for interactive yes/no prompt behavior.
// Why: Keep confirmation prompt logic deterministic and output-capturable.
package interaction

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPromptYesNoWithIO(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yes", input: "y\n", want: true},
		{name: "yes full", input: "yes\n", want: true},
		{name: "no default", input: "\n", want: false},
		{name: "other value", input: "abc\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := PromptYesNoWithIO(strings.NewReader(tt.input), &out, "Proceed?")
			if err != nil {
				t.Fatalf("prompt yes/no: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
			if !strings.Contains(out.String(), "Proceed? [y/N]: ") {
				t.Fatalf("unexpected prompt output: %q", out.String())
			}
		})
	}
}

func TestPromptYesNoWithIOReadError(t *testing.T) {
	_, err := PromptYesNoWithIO(errorReader{}, &bytes.Buffer{}, "Proceed?")
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "read confirmation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}
