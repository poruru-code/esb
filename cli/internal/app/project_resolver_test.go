// Where: cli/internal/app/project_resolver_test.go
// What: Tests for project directory resolution.
// Why: Ensure commands honor --template, ESB_PROJECT, and recent project selection.
package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestResolveProjectSelectionUsesTemplateDir(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cli := CLI{Template: templatePath}
	deps := Dependencies{ProjectDir: t.TempDir()}

	got, err := resolveProjectSelection(cli, deps, resolveOptions{})
	if err != nil {
		t.Fatalf("resolve project selection: %v", err)
	}
	if got.Dir != projectDir {
		t.Fatalf("unexpected project dir: %s", got.Dir)
	}
	absTemplate, _ := filepath.Abs(templatePath)
	if got.TemplateOverride != absTemplate {
		t.Fatalf("unexpected template override: %s", got.TemplateOverride)
	}
}

func TestResolveProjectSelectionUsesESBProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ESB_PROJECT", "demo")

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

	cli := CLI{}
	deps := Dependencies{ProjectDir: t.TempDir()}

	got, err := resolveProjectSelection(cli, deps, resolveOptions{})
	if err != nil {
		t.Fatalf("resolve project selection: %v", err)
	}
	if got.Dir != projectDir {
		t.Fatalf("unexpected project dir: %s", got.Dir)
	}
}

func TestResolveProjectSelectionUsesMostRecentProject(t *testing.T) {
	firstDir := t.TempDir()
	if err := writeGeneratorFixture(firstDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	secondDir := t.TempDir()
	if err := writeGeneratorFixture(secondDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	globalCfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: firstDir, LastUsed: "2026-01-01T00:00:00Z"},
			"beta":  {Path: secondDir, LastUsed: "2026-01-02T00:00:00Z"},
		},
	}
	if err := config.SaveGlobalConfig(configPath, globalCfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	cli := CLI{}
	deps := Dependencies{ProjectDir: t.TempDir()}

	got, err := resolveProjectSelection(cli, deps, resolveOptions{})
	if err != nil {
		t.Fatalf("resolve project selection: %v", err)
	}
	if got.Dir != secondDir {
		t.Fatalf("unexpected project dir: %s", got.Dir)
	}
}
