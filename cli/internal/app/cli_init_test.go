// Where: cli/internal/app/cli_init_test.go
// What: Tests for init command wiring.
// Why: Ensure CLI creates generator.yml through app.Run.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestRunInit(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"--template", templatePath, "init", "--env", "default,staging"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "generator.yml")); err != nil {
		t.Fatalf("expected generator.yml to exist: %v", err)
	}

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	cfg, err := config.LoadGlobalConfig(configPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	entry, ok := cfg.Projects[filepath.Base(projectDir)]
	if !ok {
		t.Fatalf("expected project entry for %s", filepath.Base(projectDir))
	}
	if entry.Path == "" {
		t.Fatalf("expected project path to be set")
	}
	if entry.LastUsed == "" {
		t.Fatalf("expected last_used to be set")
	}
}

func TestRunInitMissingTemplate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"init"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing template")
	}
}

func TestRunInitWithName(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"--template", templatePath, "init", "--name", "myapp", "--env", "dev"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	cfg, err := config.LoadGeneratorConfig(filepath.Join(projectDir, "generator.yml"))
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}
	if cfg.App.Name != "myapp" {
		t.Fatalf("unexpected app name: %s", cfg.App.Name)
	}

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	globalCfg, err := config.LoadGlobalConfig(configPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	entry, ok := globalCfg.Projects["myapp"]
	if !ok {
		t.Fatalf("expected project entry for myapp")
	}
	if entry.Path == "" {
		t.Fatalf("expected project path to be set")
	}
	if entry.LastUsed == "" {
		t.Fatalf("expected last_used to be set")
	}
}
