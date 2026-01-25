// Where: cli/internal/commands/build_test.go
// What: Tests for build command wiring.
// Why: Ensure build requests are formed correctly for build-only CLI.
package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/meta"
)

type fakeBuilder struct {
	requests []generator.BuildRequest
	err      error
}

func (f *fakeBuilder) Build(req generator.BuildRequest) error {
	f.requests = append(f.requests, req)
	return f.err
}

func setBuildEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_PREFIX", meta.EnvPrefix)
	t.Setenv(meta.EnvPrefix+"_TAG", "latest")
}

func TestRunBuildCallsBuilder(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: builder}}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging", "--mode", "docker"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	req := builder.requests[0]
	if req.Env != "staging" {
		t.Fatalf("unexpected env: %s", req.Env)
	}
	if req.Mode != "docker" {
		t.Fatalf("unexpected mode: %s", req.Mode)
	}
	if req.ProjectDir != projectDir {
		t.Fatalf("unexpected project dir: %s", req.ProjectDir)
	}
	if req.TemplatePath != templatePath {
		t.Fatalf("unexpected template path: %s", req.TemplatePath)
	}
}

func TestRunBuildMissingTemplate(t *testing.T) {
	setBuildEnv(t)
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: &fakeBuilder{}}}

	exitCode := Run([]string{"build", "--env", "staging", "--mode", "docker"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing template")
	}
}

func TestRunBuildMissingEnv(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: &fakeBuilder{}}}

	exitCode := Run([]string{"--template", templatePath, "build", "--mode", "docker"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing env")
	}
}

func TestRunBuildMissingMode(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: &fakeBuilder{}}}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing mode")
	}
}

func TestRunBuildPassesOutputFlag(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: builder}}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging", "--mode", "docker", "--output", ".out"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	if builder.requests[0].OutputDir != ".out" {
		t.Fatalf("unexpected output dir: %s", builder.requests[0].OutputDir)
	}
}

func TestRunBuildBuilderError(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	builder := &fakeBuilder{err: errors.New("boom")}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: builder}}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging", "--mode", "docker"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for builder error")
	}
}

func TestRunBuildMissingBuilder(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging", "--mode", "docker"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing builder")
	}
}

func TestRunBuildOutputsLegacySuccess(t *testing.T) {
	setBuildEnv(t)
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Build: BuildDeps{Builder: builder}}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging", "--mode", "docker"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	expected := "✓ Build complete\n"
	if out.String() != expected {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

type mockPrompter struct {
	inputs []string
}

func (m *mockPrompter) Input(_ string, _ []string) (string, error) {
	if len(m.inputs) > 0 {
		ret := m.inputs[0]
		m.inputs = m.inputs[1:]
		return ret, nil
	}
	return "", errors.New("no more inputs")
}

func (m *mockPrompter) Select(_ string, _ []string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockPrompter) SelectValue(_ string, options []interaction.SelectOption) (string, error) {
	// For confirmBuildInputs
	for _, opt := range options {
		if opt.Value == "proceed" {
			return "proceed", nil
		}
	}
	return "", nil
}

func TestPromptTemplateParameters_AllowsEmptyString(t *testing.T) {
	// Setup temporary template
	tmpDir := t.TempDir()
	templateContent := `
Parameters:
  MyStringParam:
    Type: String
    Description: A string parameter
`
	templatePath := filepath.Join(tmpDir, "template.yml")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// We provide [""] to simulate: input empty.
	mp := &mockPrompter{
		inputs: []string{""},
	}

	// Function under test
	params, err := promptTemplateParameters(templatePath, true, mp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verification
	if val, ok := params["MyStringParam"]; !ok || val != "" {
		t.Errorf("Expected final value '', got '%s' (exists: %v)", val, ok)
	}
}
