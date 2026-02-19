// Where: cli/internal/infra/interaction/interaction_test.go
// What: Tests for interactive yes/no prompt behavior.
// Why: Keep confirmation prompt logic deterministic and output-capturable.
package interaction

import (
	"bytes"
	"errors"
	"io"
	"os"
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

func TestPromptYesNoUsesDefaultStdio(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	_, _ = stdinW.Write([]byte("yes\n"))
	_ = stdinW.Close()

	oldStdin := os.Stdin
	oldStderr := os.Stderr
	os.Stdin = stdinR
	os.Stderr = stderrW
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stderr = oldStderr
		_ = stdinR.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
	})

	ok, err := PromptYesNo("Continue?")
	if err != nil {
		t.Fatalf("PromptYesNo() error = %v", err)
	}
	if !ok {
		t.Fatal("PromptYesNo() = false, want true")
	}

	_ = stderrW.Close()
	out, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if !strings.Contains(string(out), "Continue? [y/N]: ") {
		t.Fatalf("unexpected prompt output: %q", string(out))
	}
}

func TestIsTerminalNilAndPipe(t *testing.T) {
	if IsTerminal(nil) {
		t.Fatal("IsTerminal(nil) must be false")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	defer func() {
		_ = r.Close()
		_ = w.Close()
	}()
	if IsTerminal(r) {
		t.Fatal("IsTerminal(pipe) must be false")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}
