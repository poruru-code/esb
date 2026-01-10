// Where: cli/internal/app/project_add_test.go
// What: Tests for 'esb project add' command.
// Why: Ensure new project initialization and existing project registration work correctly.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunProjectAdd_New(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Should act like old 'esb init'
	exitCode := Run([]string{"--template", templatePath, "project", "add", projectDir, "-e", "prod:docker"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	// Verify generator.yml created
	genPath := filepath.Join(projectDir, "generator.yml")
	if _, err := os.Stat(genPath); os.IsNotExist(err) {
		t.Fatal("generator.yml was not created")
	}

	output := out.String()
	if !strings.Contains(output, "Configuration saved to") {
		t.Errorf("missing success message: %s", output)
	}
}

func TestRunProjectAdd_Existing(t *testing.T) {
	projectDir := t.TempDir()
	genPath := filepath.Join(projectDir, "generator.yml")
	if err := os.WriteFile(genPath, []byte("app: {name: demo}\npaths: {samTemplate: template.yml}"), 0o644); err != nil {
		t.Fatalf("failed to write generator.yml: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Should just register the existing project
	exitCode := Run([]string{"project", "add", projectDir}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "Project registered") {
		t.Errorf("missing registration message: %s", output)
	}
}

func TestRunProjectAdd_Interactive(t *testing.T) {
	// Mock isTerminal to true
	oldIsTerminal := isTerminal
	isTerminal = func(_ *os.File) bool { return true }
	defer func() { isTerminal = oldIsTerminal }()

	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Provide prompter for interaction
	mock := &mockPrompter{
		inputFn: func(title string, _ []string) (string, error) {
			if strings.Contains(title, "Environment name") {
				return "dev:containerd", nil
			}
			return "", nil
		},
	}
	deps.Prompter = mock

	// Run without environment flag
	exitCode := Run([]string{"--template", templatePath, "project", "add", projectDir}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	if !strings.Contains(out.String(), "Configuration saved") {
		t.Errorf("expected interactive flow to complete, got: %s", out.String())
	}
}

func TestRunProjectAdd_InteractiveTemplate(t *testing.T) {
	// Mock isTerminal to true
	oldIsTerminal := isTerminal
	isTerminal = func(_ *os.File) bool { return true }
	defer func() { isTerminal = oldIsTerminal }()

	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "sam.yaml") // intentionally not template.yaml
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Provide prompter for interaction
	mock := &mockPrompter{
		inputPathFn: func(title string) (string, error) {
			if strings.Contains(title, "Template path") {
				return templatePath, nil
			}
			return "", nil
		},
		inputFn: func(title string, _ []string) (string, error) {
			if strings.Contains(title, "Environment name") {
				return "dev:docker", nil
			}
			return "", nil
		},
	}
	deps.Prompter = mock

	// Run without template flag
	exitCode := Run([]string{"project", "add", projectDir}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	if !strings.Contains(out.String(), "Configuration saved") {
		t.Errorf("expected interactive flow for template to complete, got: %s", out.String())
	}
}

func TestRunProjectAdd_AutoDetectTemplate(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml") // standard name
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
	}

	// Run without template flag and with environment flag to skip environment prompt
	exitCode := Run([]string{"project", "add", projectDir, "-e", "dev:docker"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", exitCode, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "Detected template: template.yaml") {
		t.Errorf("expected auto-detection message, got: %s", output)
	}
	if !strings.Contains(output, "Configuration saved") {
		t.Errorf("expected success, got: %s", output)
	}
}
