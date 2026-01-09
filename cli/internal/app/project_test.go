// Where: cli/internal/app/project_test.go
// What: Tests for project management commands.
// Why: Ensure project list/recent/use update global config correctly.
package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestRunProjectRecentOrdersByLastUsed(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	alphaTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	betaTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: alphaTime.Format(time.RFC3339)},
			"beta":  {Path: "/projects/beta", LastUsed: betaTime.Format(time.RFC3339)},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"project", "recent"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := out.String()
	idxBeta := strings.Index(output, "beta")
	idxAlpha := strings.Index(output, "alpha")
	if idxBeta == -1 || idxAlpha == -1 || idxBeta > idxAlpha {
		t.Fatalf("unexpected order: %q", output)
	}
}

func TestRunProjectUseUpdatesGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := config.GlobalConfig{
		Version:       1,
		ActiveProject: "alpha",
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: "2026-01-01T00:00:00Z"},
			"beta":  {Path: "/projects/beta", LastUsed: "2026-01-02T00:00:00Z"},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	now := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	deps := Dependencies{
		Out: &out,
		Now: func() time.Time {
			return now
		},
	}

	exitCode := Run([]string{"project", "use", "beta"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	updated, err := config.LoadGlobalConfig(configPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if updated.ActiveProject != "beta" {
		t.Fatalf("unexpected active project: %s", updated.ActiveProject)
	}
	if entry := updated.Projects["beta"]; entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("unexpected last_used: %s", entry.LastUsed)
	}
}

func TestRunProjectUseByIndex(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	alphaTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	betaTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: alphaTime.Format(time.RFC3339)},
			"beta":  {Path: "/projects/beta", LastUsed: betaTime.Format(time.RFC3339)},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	deps := Dependencies{
		Out: &out,
		Now: func() time.Time {
			return now
		},
	}

	exitCode := Run([]string{"project", "use", "1"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	updated, err := config.LoadGlobalConfig(configPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if updated.ActiveProject != "beta" {
		t.Fatalf("unexpected active project: %s", updated.ActiveProject)
	}
}

func TestRunProjectListOutputsNames(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: "2026-01-01T00:00:00Z"},
			"beta":  {Path: "/projects/beta", LastUsed: "2026-01-02T00:00:00Z"},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"project", "list"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := out.String()
	if !strings.Contains(output, "alpha") || !strings.Contains(output, "beta") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunProjectUseRejectsUnknownProject(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: "2026-01-01T00:00:00Z"},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"project", "use", "unknown"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for unknown project")
	}
}

func TestProjectRecentFormatsIndex(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {Path: "/projects/alpha", LastUsed: "2026-01-01T00:00:00Z"},
		},
	}
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if err := config.SaveGlobalConfig(configPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"project", "recent"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := out.String()
	if !strings.Contains(output, "1.") || !strings.Contains(output, "alpha") {
		t.Fatalf("unexpected output: %q", output)
	}
}
