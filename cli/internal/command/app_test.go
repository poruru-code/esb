// Where: cli/internal/command/app_test.go
// What: Tests for CLI run behavior.
// Why: Ensure command routing remains stable.
package command

import (
	"bytes"
	"strings"
	"testing"
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
