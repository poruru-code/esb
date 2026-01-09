// Where: cli/internal/app/app_test.go
// What: Tests for CLI run behavior.
// Why: Ensure status command wiring is stable.
package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type fakeDetector struct {
	state state.State
	err   error
}

func (f fakeDetector) Detect() (state.State, error) {
	return f.state, f.err
}

func TestRunStatus(t *testing.T) {
	var out bytes.Buffer
	deps := Dependencies{
		Out: &out,
		DetectorFactory: func(_, _ string) (StateDetector, error) {
			return fakeDetector{state: state.StateRunning}, nil
		},
		ProjectDir: "/tmp/project",
	}

	exitCode := Run([]string{"status"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := out.String(); got == "" || got == "\n" {
		t.Fatalf("expected output, got %q", got)
	}
	if !strings.Contains(out.String(), "running") {
		t.Fatalf("expected output to include running, got %q", out.String())
	}
}

func TestRunStatusDetectError(t *testing.T) {
	var out bytes.Buffer
	deps := Dependencies{
		Out: &out,
		DetectorFactory: func(_, _ string) (StateDetector, error) {
			return fakeDetector{err: errors.New("boom")}, nil
		},
		ProjectDir: "/tmp/project",
	}

	exitCode := Run([]string{"status"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code on error")
	}
}

func TestRunStatusFactoryError(t *testing.T) {
	deps := Dependencies{
		DetectorFactory: func(_, _ string) (StateDetector, error) {
			return nil, errors.New("factory")
		},
		ProjectDir: "/tmp/project",
	}

	exitCode := Run([]string{"status"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code on factory error")
	}
}

func TestRunStatusUsesActiveEnvFromGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	globalCfg := config.GlobalConfig{
		Version:       1,
		ActiveProject: "demo",
		ActiveEnvironments: map[string]string{
			"demo": "staging",
		},
		Projects: map[string]config.ProjectEntry{
			"demo": {Path: projectDir},
		},
	}
	if err := config.SaveGlobalConfig(configPath, globalCfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var capturedEnv string
	deps := Dependencies{
		DetectorFactory: func(_, env string) (StateDetector, error) {
			capturedEnv = env
			return fakeDetector{state: state.StateRunning}, nil
		},
		ProjectDir: projectDir,
	}

	exitCode := Run([]string{"status"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if capturedEnv != "staging" {
		t.Fatalf("unexpected env: %s", capturedEnv)
	}
}

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
