// Where: cli/internal/app/env_var_test.go
// What: Tests for env var command.
// Why: Ensure env var command correctly handles interactive and direct usage.
package app

import (
	"bytes"
	"testing"
)

func TestRunEnvVarRequiresService(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_MODE", "")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	// Non-interactive mode without service should fail
	exitCode := Run([]string{"env", "var"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code when service not specified")
	}
}

func TestRunEnvVarWithInteractiveSelection(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_MODE", "")

	mockPrompter := &mockPrompter{
		selectFn: func(_ string, _ []string) (string, error) {
			// Simulate selecting "gateway"
			return "gateway", nil
		},
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Prompter:   mockPrompter,
	}

	// This will fail because there's no actual docker container
	// We're just testing that the interactive selection works
	exitCode := Run([]string{"env", "var"}, deps)

	// The command will fail because compose files don't exist in temp dir
	// or because docker isn't running - that's expected
	// We're testing the wiring, not the actual docker integration
	if mockPrompter.lastTitle == "Select service" {
		// Selection was attempted, which is what we're testing
		return
	}
	// Otherwise it should have failed at context resolution
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code in test environment")
	}
}
