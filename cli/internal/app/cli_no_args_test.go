// Where: cli/internal/app/cli_no_args_test.go
// What: Tests for project/env root commands without subcommands.
// Why: Ensure 'esb env' and 'esb project' reflect 'list' behavior.
package app

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestRunEnvRoot(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Should behave like 'env list'
	exitCode := Run([]string{"env"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "default") {
		t.Fatalf("expected output to contain 'default', got: %s", output)
	}
}

func TestRunProjectRoot(t *testing.T) {
	projectDir := t.TempDir()
	setupProjectConfig(t, projectDir, "demo")

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	os.Setenv("ESB_PROJECT", "demo")
	defer os.Unsetenv("ESB_PROJECT")

	// Should behave like 'project list'
	exitCode := Run([]string{"project"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "📦  demo") {
		t.Fatalf("expected output to contain '📦  demo', got: %s", output)
	}
}
