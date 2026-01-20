// Where: cli/internal/commands/app_test.go
// What: Tests for CLI run behavior.
// Why: Ensure command routing remains stable.
package commands

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

func TestRunProjectRemove_NoArgs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	setupProjectConfig(t, projectDir, "demo-project")

	var out bytes.Buffer
	prompter := &mockPrompter{
		selectFn: func(_ string, options []string) (string, error) {
			return options[0], nil
		},
	}

	deps := Dependencies{
		Out:      &out,
		Prompter: prompter,
	}

	exitCode := Run([]string{"project", "remove"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(out.String(), "Removed project 'demo-project'") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
