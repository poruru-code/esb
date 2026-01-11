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
	if entry := updated.Projects["beta"]; entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("unexpected last_used: %s", entry.LastUsed)
	}
	if !strings.Contains(out.String(), "export ESB_PROJECT=beta") {
		t.Fatalf("unexpected output: %q", out.String())
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
	if entry := updated.Projects["beta"]; entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("unexpected last_used: %s", entry.LastUsed)
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

func TestRunProjectRemove(t *testing.T) {
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

	// Remove by name
	exitCode := Run([]string{"project", "remove", "alpha"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	updated, _ := config.LoadGlobalConfig(configPath)
	if _, ok := updated.Projects["alpha"]; ok {
		t.Fatalf("expected alpha to be removed")
	}

	// Remove by selection (interactive)
	prompter := &mockPrompter{
		selectedValue: "beta",
	}
	deps.Prompter = prompter
	exitCode = Run([]string{"project", "remove"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	updated, _ = config.LoadGlobalConfig(configPath)
	if _, ok := updated.Projects["beta"]; ok {
		t.Fatalf("expected beta to be removed")
	}
}

func TestRunProjectUseInteractive(t *testing.T) {
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

	prompter := &mockPrompter{
		selectedValue: "alpha",
	}
	var out bytes.Buffer
	deps := Dependencies{
		Out:      &out,
		Prompter: prompter,
		Now:      func() time.Time { return time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC) },
	}

	exitCode := Run([]string{"project", "use"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(out.String(), "export ESB_PROJECT=alpha") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestSelectProject_NumericNameCollision(t *testing.T) {
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"001":          {Path: "/projects/001", LastUsed: "2026-01-01T00:00:00Z"},
			"e2e-fixtures": {Path: "/projects/e2e", LastUsed: "2026-01-02T00:00:00Z"},
		},
	}

	// In this case, "e2e-fixtures" is index 1 (most recent), "001" is index 2.
	// If we select "001" as a name, it should resolve to "001", not index 1 ("e2e-fixtures").

	name, err := selectProject(cfg, "001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "001" {
		t.Errorf("expected 001, got %s", name)
	}
}
