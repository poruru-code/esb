// Where: cli/internal/app/info_test.go
// What: Tests for info command output.
// Why: Ensure info reports config, environment, and state clearly.
package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type fakeInfoDetector struct {
	state state.State
	err   error
}

func (f fakeInfoDetector) Detect() (state.State, error) {
	return f.state, f.err
}

func TestRunInfoOutputsConfigAndState(t *testing.T) {
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
	t.Setenv("ESB_PROJECT", "demo")
	t.Setenv("ESB_ENV", "staging")

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	globalCfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"demo": {Path: projectDir},
		},
	}
	if err := config.SaveGlobalConfig(configPath, globalCfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var capturedEnv string
	deps := Dependencies{
		ProjectDir: projectDir,
		DetectorFactory: func(_, env string) (StateDetector, error) {
			capturedEnv = env
			return fakeInfoDetector{state: state.StateRunning}, nil
		},
	}

	var out bytes.Buffer
	deps.Out = &out

	// Call with no args (equivalent to old 'info' command)
	exitCode := Run([]string{}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	if capturedEnv != "staging" {
		t.Fatalf("unexpected env: %s", capturedEnv)
	}

	output := out.String()
	if !strings.Contains(output, "config.yaml") {
		t.Fatalf("expected config path in output: %q", output)
	}
	if !strings.Contains(output, "name:   staging (containerd)") {
		t.Fatalf("expected env in output: %q", output)
	}
	if !strings.Contains(output, "state:  running") {
		t.Fatalf("expected state in output: %q", output)
	}
}
