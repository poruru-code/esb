// Where: cli/internal/app/env_test.go
// What: Tests for environment management commands.
// Why: Ensure env list/use/create/remove update generator.yml and global config.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
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

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

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

	if cfg.ActiveProject != "demo" {
		t.Fatalf("unexpected active project: %s", cfg.ActiveProject)
	}
	if cfg.ActiveEnvironments["demo"] != "staging" {
		t.Fatalf("unexpected active env: %s", cfg.ActiveEnvironments["demo"])
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
}

func TestRunEnvCreateAddsEnvironment(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"env", "create", "staging"}, deps)
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

func TestRunEnvRemoveRejectsLastEnvironment(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

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
