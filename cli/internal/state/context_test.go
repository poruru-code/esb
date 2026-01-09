// Where: cli/internal/state/context_test.go
// What: Tests for resolving generator context.
// Why: Ensure environment scoping and path resolution are deterministic.
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveContext_MissingGenerator(t *testing.T) {
	projectDir := t.TempDir()
	_, err := ResolveContext(projectDir, "default")
	if err == nil {
		t.Fatalf("expected error when generator.yml is missing")
	}
}

func TestResolveContext_InvalidEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorYml(projectDir, []string{"default"}, "./template.yaml", ".esb/"); err != nil {
		t.Fatalf("write generator.yml: %v", err)
	}
	if err := writeFile(filepath.Join(projectDir, "template.yaml")); err != nil {
		t.Fatalf("write template.yaml: %v", err)
	}

	_, err := ResolveContext(projectDir, "staging")
	if err == nil {
		t.Fatalf("expected error when env is not registered")
	}
}

func TestResolveContext_ResolvesPaths(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorYml(projectDir, []string{"default"}, "./template.yaml", ".esb/"); err != nil {
		t.Fatalf("write generator.yml: %v", err)
	}
	if err := writeFile(filepath.Join(projectDir, "template.yaml")); err != nil {
		t.Fatalf("write template.yaml: %v", err)
	}

	ctx, err := ResolveContext(projectDir, "default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if ctx.TemplatePath != expectedTemplate {
		t.Fatalf("template path mismatch: got %s", ctx.TemplatePath)
	}
	if ctx.OutputDir != filepath.Join(projectDir, ".esb") {
		t.Fatalf("output dir mismatch: got %s", ctx.OutputDir)
	}
	if ctx.OutputEnvDir != filepath.Join(projectDir, ".esb", "default") {
		t.Fatalf("output env dir mismatch: got %s", ctx.OutputEnvDir)
	}
	if ctx.ComposeProject != "esb-default" {
		t.Fatalf("compose project mismatch: got %s", ctx.ComposeProject)
	}
}

func TestResolveContext_ModeFromMap(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorYmlMap(projectDir, map[string]string{"default": "containerd"}, "./template.yaml", ".esb/"); err != nil {
		t.Fatalf("write generator.yml: %v", err)
	}
	if err := writeFile(filepath.Join(projectDir, "template.yaml")); err != nil {
		t.Fatalf("write template.yaml: %v", err)
	}

	ctx, err := ResolveContext(projectDir, "default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ctx.Mode != "containerd" {
		t.Fatalf("expected mode containerd, got %s", ctx.Mode)
	}
}

func writeGeneratorYml(projectDir string, envs []string, template, outputDir string) error {
	content := "environments:\n"
	for _, env := range envs {
		content += "  - " + env + "\n"
	}
	content += "paths:\n"
	content += "  sam_template: " + template + "\n"
	content += "  output_dir: " + outputDir + "\n"

	path := filepath.Join(projectDir, "generator.yml")
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeGeneratorYmlMap(projectDir string, envs map[string]string, template, outputDir string) error {
	content := "environments:\n"
	for env, mode := range envs {
		content += "  " + env + ": " + mode + "\n"
	}
	content += "paths:\n"
	content += "  sam_template: " + template + "\n"
	content += "  output_dir: " + outputDir + "\n"

	path := filepath.Join(projectDir, "generator.yml")
	return os.WriteFile(path, []byte(content), 0o644)
}
