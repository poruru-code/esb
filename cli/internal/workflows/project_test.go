// Where: cli/internal/workflows/project_test.go
// What: Unit tests for project workflows.
// Why: Validate project list/recent/use/remove/register behavior without CLI adapters.
package workflows

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestProjectListWorkflowActiveSorted(t *testing.T) {
	t.Setenv("ESB_PROJECT", "bravo")
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"bravo": {Path: "/bravo"},
			"alpha": {Path: "/alpha"},
		},
	}

	result, err := NewProjectListWorkflow().Run(ProjectListRequest{Config: cfg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.Projects))
	}
	if result.Projects[0].Name != "alpha" || result.Projects[1].Name != "bravo" {
		t.Fatalf("expected projects sorted by name")
	}
	if result.Projects[1].Active != true {
		t.Fatalf("expected bravo to be active")
	}
}

func TestProjectRecentWorkflowOrder(t *testing.T) {
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {LastUsed: "2025-01-01T00:00:00Z"},
			"bravo": {LastUsed: "2025-01-02T00:00:00Z"},
		},
	}

	result, err := NewProjectRecentWorkflow().Run(ProjectRecentRequest{Config: cfg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.Projects))
	}
	if result.Projects[0].Name != "bravo" {
		t.Fatalf("expected most recent project first")
	}
}

func TestProjectUseWorkflowUpdatesLastUsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"demo": {Path: "/old"},
		},
	}
	if err := config.SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save global: %v", err)
	}

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := NewProjectUseWorkflow().Run(ProjectUseRequest{
		ProjectName:      "demo",
		GlobalConfig:     cfg,
		GlobalConfigPath: path,
		Now:              now,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := config.LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	entry := updated.Projects["demo"]
	if entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("expected last used timestamp to be updated")
	}
}

func TestProjectUseWorkflowRequiresPath(t *testing.T) {
	err := NewProjectUseWorkflow().Run(ProjectUseRequest{ProjectName: "demo"})
	if err == nil || !strings.Contains(err.Error(), "global config path not available") {
		t.Fatalf("expected missing path error, got %v", err)
	}
}

func TestProjectRemoveWorkflowRemovesProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"demo":  {Path: "/demo"},
			"other": {Path: "/other"},
		},
	}
	if err := config.SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save global: %v", err)
	}

	if err := NewProjectRemoveWorkflow().Run(ProjectRemoveRequest{
		ProjectName:      "demo",
		GlobalConfig:     cfg,
		GlobalConfigPath: path,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := config.LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	if _, ok := updated.Projects["demo"]; ok {
		t.Fatalf("expected demo project to be removed")
	}
}

func TestProjectRegisterWorkflowCreatesEntry(t *testing.T) {
	projectDir := t.TempDir()
	genPath := filepath.Join(projectDir, "generator.yml")
	genCfg := config.GeneratorConfig{
		App:          config.AppConfig{Name: "demo"},
		Environments: config.Environments{{Name: "dev", Mode: "docker"}},
		Paths:        config.PathsConfig{SamTemplate: "template.yaml", OutputDir: ".esb/"},
	}
	if err := config.SaveGeneratorConfig(genPath, genCfg); err != nil {
		t.Fatalf("save generator: %v", err)
	}

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("ESB_CONFIG_PATH", globalPath)

	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if err := NewProjectRegisterWorkflow().Run(ProjectRegisterRequest{
		GeneratorPath: genPath,
		Now:           now,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := config.LoadGlobalConfig(globalPath)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	entry, ok := updated.Projects["demo"]
	if !ok {
		t.Fatalf("expected demo project to be registered")
	}
	if entry.Path != projectDir {
		t.Fatalf("expected project path to be set")
	}
	if entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("expected last used timestamp to be set")
	}
}
