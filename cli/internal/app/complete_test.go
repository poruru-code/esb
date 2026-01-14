// Where: cli/internal/app/complete_test.go
// What: Tests for completion candidate helpers.
// Why: Ensure dynamic completion outputs expected names.
package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestRunCompleteEnvOutputsEnvironments(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}
	exitCode := Run([]string{"__complete", "env"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.Fields(out.String())
	if len(got) != 2 || got[0] != "default" || got[1] != "staging" {
		t.Fatalf("unexpected env list: %v", got)
	}
}

func TestRunCompleteProjectOutputsProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"beta":  {Path: "/tmp/beta"},
			"alpha": {Path: "/tmp/alpha"},
		},
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var out bytes.Buffer
	exitCode := Run([]string{"__complete", "project"}, Dependencies{Out: &out})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.Fields(out.String())
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("unexpected project list: %v", got)
	}
}
