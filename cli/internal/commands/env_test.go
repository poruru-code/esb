// Where: cli/internal/commands/env_test.go
// What: Tests for environment management commands.
// Why: Ensure env list/use/create/remove update generator.yml and global config.
package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
)

func TestRunEnvList(t *testing.T) {
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

	exitCode := Run([]string{"env", "list"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := out.String()
	if !strings.Contains(output, "default") || !strings.Contains(output, "staging") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunEnvUseUpdatesGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	setupProjectConfig(t, projectDir, "demo")

	now := time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Now: func() time.Time {
			return now
		},
	}

	exitCode := Run([]string{"env", "use", "staging"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	cfg, err := config.LoadGlobalConfig(configPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		t.Fatalf("abs project dir: %v", err)
	}
	entry, ok := cfg.Projects["demo"]
	if !ok {
		t.Fatalf("expected project entry for demo")
	}
	if entry.Path != absProjectDir {
		t.Fatalf("unexpected project path: %s", entry.Path)
	}
	if entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("unexpected last_used: %s", entry.LastUsed)
	}
	if !strings.Contains(out.String(), "export ESB_ENV=staging") {
		t.Fatalf("unexpected output: %q", out.String())
	}

	generatorCfg, err := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}
	if generatorCfg.App.LastEnv != "staging" {
		t.Fatalf("unexpected last_env: %s", generatorCfg.App.LastEnv)
	}
}

func TestRunEnvCreateAddsEnvironment(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"env", "add", "staging"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	cfg, err := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}
	if !cfg.Environments.Has("staging") {
		t.Fatalf("expected staging env to exist")
	}
	mode, _ := cfg.Environments.Mode("staging")
	if mode == "" {
		t.Fatalf("expected mode to be set")
	}
}

func TestRunEnvRemoveDeletesEnvironment(t *testing.T) {
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

	exitCode := Run([]string{"env", "remove", "staging"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	cfg, err := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}
	if cfg.Environments.Has("staging") {
		t.Fatalf("expected staging env to be removed")
	}
	if !cfg.Environments.Has("default") {
		t.Fatalf("expected default env to remain")
	}
}

func TestRunEnvCreateInteractive(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	// Mock isTerminal to return true for interactive tests
	originalIsTerminal := interaction.IsTerminal
	interaction.IsTerminal = func(_ *os.File) bool { return true }
	defer func() { interaction.IsTerminal = originalIsTerminal }()

	tests := []struct {
		name         string
		args         []string
		input        string
		selection    string
		wantEnv      string
		wantMode     string
		wantExitCode int
	}{
		{
			name:         "prompt for name and mode",
			args:         []string{"env", "add"},
			input:        "staging",
			selection:    "containerd",
			wantEnv:      "staging",
			wantMode:     "containerd",
			wantExitCode: 0,
		},
		{
			name:         "prompt for mode only",
			args:         []string{"env", "add", "prod"},
			selection:    "firecracker",
			wantEnv:      "prod",
			wantMode:     "firecracker",
			wantExitCode: 0,
		},
		{
			name:         "no prompt when name and mode provided",
			args:         []string{"env", "add", "dev:docker"},
			wantEnv:      "dev",
			wantMode:     "docker",
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			prompter := &mockPrompter{
				inputFn: func(_ string, _ []string) (string, error) {
					return tt.input, nil
				},
				selectFn: func(_ string, _ []string) (string, error) {
					return tt.selection, nil
				},
			}
			deps := Dependencies{
				Out:        &out,
				ProjectDir: projectDir,
				Prompter:   prompter,
			}

			exitCode := Run(tt.args, deps)
			if exitCode != tt.wantExitCode {
				t.Fatalf("expected exit code %d, got %d; output: %s", tt.wantExitCode, exitCode, out.String())
			}

			if tt.wantExitCode == 0 {
				cfg, err := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
				if err != nil {
					t.Fatalf("load generator config: %v", err)
				}
				if !cfg.Environments.Has(tt.wantEnv) {
					t.Fatalf("expected %s env to exist", tt.wantEnv)
				}
				mode, _ := cfg.Environments.Mode(tt.wantEnv)
				if mode != tt.wantMode {
					t.Fatalf("expected mode %s, got %s", tt.wantMode, mode)
				}
			}
		})
	}
}

func TestRunEnvRemoveRejectsLastEnvironment(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"env", "remove", "default"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code when removing last env")
	}
}

func writeGeneratorFixtureWithEnvs(projectDir string, envs config.Environments, appName string) error {
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		return err
	}

	cfg := config.GeneratorConfig{
		App: config.AppConfig{
			Name: appName,
		},
		Environments: envs,
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
	}
	return config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg)
}

func setupProjectConfig(t *testing.T, projectDir, name string) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ESB_PROJECT", name)
	t.Setenv("ESB_ENV", "")

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			name: {Path: projectDir, LastUsed: "2026-01-01T00:00:00Z"},
		},
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}
}

func TestRunEnvUseInteractive(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	prompter := &mockPrompter{
		selectedValue: "staging",
	}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Prompter:   prompter,
	}

	exitCode := Run([]string{"env", "use"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if prompter.lastTitle != "Select environment" {
		t.Fatalf("unexpected prompt title: %s", prompter.lastTitle)
	}

	if !strings.Contains(out.String(), "export ESB_ENV=staging") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunEnvRemoveInteractive(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	prompter := &mockPrompter{
		selectedValue: "staging",
	}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Prompter:   prompter,
	}

	exitCode := Run([]string{"env", "remove"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if prompter.lastTitle != "Select environment to remove" {
		t.Fatalf("unexpected prompt title: %s", prompter.lastTitle)
	}

	cfg, _ := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
	if cfg.Environments.Has("staging") {
		t.Fatalf("expected staging env to be removed")
	}
}
