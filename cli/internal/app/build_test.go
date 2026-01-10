// Where: cli/internal/app/build_test.go
// What: Tests for build command wiring.
// Why: Ensure build requests are formed correctly.
package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type fakeBuilder struct {
	requests []BuildRequest
	err      error
}

func (f *fakeBuilder) Build(req BuildRequest) error {
	f.requests = append(f.requests, req)
	return f.err
}

func TestRunBuildCallsBuilder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	templatePath := filepath.Join(projectDir, "template.yaml")

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: builder}

	exitCode := Run([]string{"--template", templatePath, "build", "--env", "staging"}, deps)
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
	if req.ProjectDir != projectDir {
		t.Fatalf("unexpected project dir: %s", req.ProjectDir)
	}
	if req.TemplatePath != templatePath {
		t.Fatalf("unexpected template path: %s", req.TemplatePath)
	}
}

func TestRunBuildMissingTemplate(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: &fakeBuilder{}, ProjectDir: projectDir}

	exitCode := Run([]string{"build"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing template")
	}
}

func TestRunBuildBuilderError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	templatePath := filepath.Join(projectDir, "template.yaml")

	builder := &fakeBuilder{err: errors.New("boom")}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: builder}

	exitCode := Run([]string{"--template", templatePath, "build"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for builder error")
	}
}

func TestRunBuildMissingBuilder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out}

	exitCode := Run([]string{"--template", templatePath, "build"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing builder")
	}
}

func TestRunBuildUsesActiveEnvFromGlobalConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	templatePath := filepath.Join(projectDir, "template.yaml")
	t.Setenv("ESB_ENV", "staging")

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: builder}

	exitCode := Run([]string{"--template", templatePath, "build"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	if builder.requests[0].Env != "staging" {
		t.Fatalf("unexpected env: %s", builder.requests[0].Env)
	}
}

func TestRunBuildUsesGeneratorTemplateWhenTemplateFlagMissing(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: builder, ProjectDir: projectDir}

	exitCode := Run([]string{"build"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if builder.requests[0].TemplatePath != expectedTemplate {
		t.Fatalf("unexpected template path: %s", builder.requests[0].TemplatePath)
	}
}

func TestRunBuildPassesNoCacheFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	templatePath := filepath.Join(projectDir, "template.yaml")

	builder := &fakeBuilder{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, Builder: builder, ProjectDir: projectDir}

	exitCode := Run([]string{"--template", templatePath, "build", "--no-cache"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	if !builder.requests[0].NoCache {
		t.Fatalf("expected no-cache to be true")
	}
}
