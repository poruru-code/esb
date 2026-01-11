// Where: cli/internal/app/command_context_test.go
// What: Tests for shared command context resolution.
// Why: Ensure environment and project selection logic is correct, including interactive flows.
package app

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestResolveCommandContext_Interactive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()

	// Prepare generator with multiple environments
	envs := config.Environments{
		{Name: "dev", Mode: "docker"},
		{Name: "prod", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	// Mock prompter to select "prod"
	prompter := &mockPrompter{
		selectValueFn: func(title string, options []selectOption) (string, error) {
			if title != "Select environment" {
				t.Errorf("unexpected title: %s", title)
			}
			if len(options) != 2 {
				t.Fatalf("expected 2 options, got %d", len(options))
			}
			return "prod", nil
		},
	}

	cli := CLI{} // No env flag
	deps := Dependencies{
		ProjectDir: projectDir,
		Prompter:   prompter,
	}
	opts := resolveOptions{
		Interactive: true,
	}

	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		t.Fatalf("resolveCommandContext failed: %v", err)
	}

	if ctxInfo.Env != "prod" {
		t.Errorf("expected env prod, got %s", ctxInfo.Env)
	}
	if ctxInfo.Context.Mode != "containerd" {
		t.Errorf("expected mode containerd, got %s", ctxInfo.Context.Mode)
	}
}

func TestResolveCommandContext_EnvFlagOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()

	envs := config.Environments{
		{Name: "dev", Mode: "docker"},
		{Name: "prod", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	cli := CLI{EnvFlag: "dev"}
	deps := Dependencies{
		ProjectDir: projectDir,
	}
	opts := resolveOptions{}

	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		t.Fatalf("resolveCommandContext failed: %v", err)
	}

	if ctxInfo.Env != "dev" {
		t.Errorf("expected env dev, got %s", ctxInfo.Env)
	}
}

func TestResolveCommandContext_AllowMissingEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()

	// Create generator with NO environments
	if err := writeGeneratorFixtureWithEnvs(projectDir, nil, "demo"); err != nil {
		t.Fatalf("write generator: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	cli := CLI{}
	deps := Dependencies{
		ProjectDir: projectDir,
	}

	// Test without AllowMissingEnv -> should error
	_, err := resolveCommandContext(cli, deps, resolveOptions{AllowMissingEnv: false})
	if err == nil {
		t.Error("expected error for missing environment")
	}

	// Test with AllowMissingEnv -> should succeed with empty env
	ctxInfo, err := resolveCommandContext(cli, deps, resolveOptions{AllowMissingEnv: true})
	if err != nil {
		t.Fatalf("resolveCommandContext failed with AllowMissingEnv: %v", err)
	}
	if ctxInfo.Env != "" {
		t.Errorf("expected empty env, got %s", ctxInfo.Env)
	}
}
