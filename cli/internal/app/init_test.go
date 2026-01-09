// Where: cli/internal/app/init_test.go
// What: Tests for init command helpers.
// Why: Ensure generator.yml is created correctly.
package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestBuildGeneratorConfig(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	cfg, generatorPath, err := buildGeneratorConfig(templatePath, envs, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if generatorPath != filepath.Join(projectDir, "generator.yml") {
		t.Fatalf("unexpected generator path: %s", generatorPath)
	}
	if cfg.Paths.SamTemplate != "template.yaml" {
		t.Fatalf("expected relative template path, got %s", cfg.Paths.SamTemplate)
	}
	if cfg.Paths.OutputDir != ".esb/" {
		t.Fatalf("expected output dir .esb/, got %s", cfg.Paths.OutputDir)
	}
	if !cfg.Environments.Has("default") || !cfg.Environments.Has("staging") {
		t.Fatalf("unexpected environments: %#v", cfg.Environments)
	}
	if cfg.App.Tag != "default" {
		t.Fatalf("expected tag default, got %s", cfg.App.Tag)
	}
	if cfg.App.Name != filepath.Base(projectDir) {
		t.Fatalf("expected app name %s, got %s", filepath.Base(projectDir), cfg.App.Name)
	}
}

func TestRunInitWritesGenerator(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	generatorPath, err := runInit(templatePath, []string{"default"}, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	loaded, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}
	if !loaded.Environments.Has("default") {
		t.Fatalf("unexpected environments: %#v", loaded.Environments)
	}
	if mode, ok := loaded.Environments.Mode("default"); !ok || mode != "docker" {
		t.Fatalf("unexpected mode: %q", mode)
	}
	if loaded.App.Name != filepath.Base(projectDir) {
		t.Fatalf("unexpected app name: %s", loaded.App.Name)
	}
}

func TestParseEnvSpecs(t *testing.T) {
	t.Setenv("ESB_MODE", "docker")
	envs, err := parseEnvSpecs([]string{"default:containerd", "staging"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mode, _ := envs.Mode("default"); mode != "containerd" {
		t.Fatalf("unexpected mode for default: %s", mode)
	}
	if mode, _ := envs.Mode("staging"); mode != "docker" {
		t.Fatalf("unexpected mode for staging: %s", mode)
	}
}

func TestBuildGeneratorConfigWithName(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	envs := config.Environments{
		{Name: "default", Mode: "docker"},
	}
	cfg, _, err := buildGeneratorConfig(templatePath, envs, "myapp")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.App.Name != "myapp" {
		t.Fatalf("expected app name myapp, got %s", cfg.App.Name)
	}
}
