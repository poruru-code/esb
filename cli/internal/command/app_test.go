// Where: cli/internal/command/app_test.go
// What: Tests for CLI run behavior.
// Why: Ensure command routing remains stable.
package command

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

func TestRunNodeDisabled(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"node", "add"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for disabled node command")
	}
	if !strings.Contains(out.String(), "node command is disabled") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunNodeDisabledWithGlobalFlags(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"--env", "staging", "--template", "template.yaml", "node", "up"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for disabled node command")
	}
	if !strings.Contains(out.String(), "node command is disabled") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCompletionCommandRemoved(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"completion", "bash"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for removed completion command")
	}
	output := strings.ToLower(out.String())
	if !strings.Contains(output, "unknown") &&
		!strings.Contains(output, "expected one of") &&
		!strings.Contains(output, "unexpected argument") {
		t.Fatalf("unexpected output for removed completion command: %q", out.String())
	}
}

func TestDispatchCommandVersion(t *testing.T) {
	var out bytes.Buffer
	exitCode, handled := dispatchCommand("version", CLI{}, Dependencies{}, &out)
	if !handled {
		t.Fatal("expected version command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), version.GetVersion()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestDispatchCommandUnknown(t *testing.T) {
	exitCode, handled := dispatchCommand("unknown", CLI{}, Dependencies{}, &bytes.Buffer{})
	if handled {
		t.Fatal("expected unknown command to be unhandled")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestHandleParseErrorTemplateFlag(t *testing.T) {
	var out bytes.Buffer
	exitCode := handleParseError(nil, errors.New("expected string value for --template"), Dependencies{}, &out)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	output := out.String()
	if !strings.Contains(output, "`-t/--template` expects a value") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestHandleParseErrorFallback(t *testing.T) {
	var out bytes.Buffer
	exitCode := handleParseError(nil, errors.New("boom"), Dependencies{}, &out)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(out.String(), "boom") {
		t.Fatalf("expected error message in output, got %q", out.String())
	}
}

func TestHandleParseErrorKnownFlags(t *testing.T) {
	tests := []string{"--env", "--mode", "--env-file"}
	for _, flag := range tests {
		t.Run(flag, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := handleParseError(nil, errors.New("expected string value for "+flag), Dependencies{}, &out)
			if exitCode != 1 {
				t.Fatalf("expected exit code 1, got %d", exitCode)
			}
			if !strings.Contains(out.String(), "expects a value") {
				t.Fatalf("unexpected output: %q", out.String())
			}
		})
	}
}

func TestRunNoArgsShowsUsage(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run(nil, Dependencies{Out: &out, ErrOut: &out})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage output, got %q", out.String())
	}
}

func TestRunVersionCommand(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"version"}, Dependencies{Out: &out, ErrOut: &out})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), version.GetVersion()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}
