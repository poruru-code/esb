// Where: cli/internal/app/reset_test.go
// What: Tests for reset command wiring.
// Why: Ensure reset orchestrates down + build.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type fakeResetDowner struct {
	calls        int
	projects     []string
	removeVolume []bool
}

func (f *fakeResetDowner) Down(project string, removeVolumes bool) error {
	f.calls++
	f.projects = append(f.projects, project)
	f.removeVolume = append(f.removeVolume, removeVolumes)
	return nil
}

type fakeResetBuilder struct {
	requests []BuildRequest
}

func (f *fakeResetBuilder) Build(request BuildRequest) error {
	f.requests = append(f.requests, request)
	return nil
}

type fakeResetUpper struct {
	calls    int
	requests []UpRequest
}

func (f *fakeResetUpper) Up(request UpRequest) error {
	f.calls++
	f.requests = append(f.requests, request)
	return nil
}

func TestRunResetCallsDownBuildUp(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_MODE", "")

	downer := &fakeResetDowner{}
	builder := &fakeResetBuilder{}
	upper := &fakeResetUpper{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Downer:     downer,
		Builder:    builder,
		Upper:      upper,
	}

	exitCode := Run([]string{"reset", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if downer.calls != 1 {
		t.Fatalf("expected downer called once, got %d", downer.calls)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
	if len(downer.removeVolume) != 1 || !downer.removeVolume[0] {
		t.Fatalf("expected remove volumes, got %v", downer.removeVolume)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected build called once, got %d", len(builder.requests))
	}
	req := builder.requests[0]
	if req.ProjectDir != projectDir {
		t.Fatalf("unexpected project dir: %s", req.ProjectDir)
	}
	if req.Env != "default" {
		t.Fatalf("unexpected env: %s", req.Env)
	}
	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if req.TemplatePath != expectedTemplate {
		t.Fatalf("unexpected template path: %s", req.TemplatePath)
	}
	if upper.calls != 1 {
		t.Fatalf("expected up called once, got %d", upper.calls)
	}
	if len(upper.requests) != 1 || upper.requests[0].Context.ComposeProject != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected up context: %v", upper.requests)
	}
	if !upper.requests[0].Detach {
		t.Fatalf("expected reset to detach")
	}
}

func TestRunResetWithoutYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv("ESB_MODE", "")

	downer := &fakeResetDowner{}
	builder := &fakeResetBuilder{}
	upper := &fakeResetUpper{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Downer:     downer,
		Builder:    builder,
		Upper:      upper,
	}

	exitCode := Run([]string{"reset"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without --yes")
	}
	if downer.calls != 0 || len(builder.requests) != 0 || upper.calls != 0 {
		t.Fatalf("expected no calls without confirmation")
	}
}

func TestRunResetMissingDeps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv("ESB_MODE", "")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"reset", "--yes"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without deps")
	}
}

func TestRunResetSetsModeFromGenerator(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixtureWithMode(projectDir, "default", "firecracker"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	t.Setenv("ESB_MODE", "")

	downer := &fakeResetDowner{}
	builder := &fakeResetBuilder{}
	upper := &fakeResetUpper{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Downer:     downer,
		Builder:    builder,
		Upper:      upper,
	}

	exitCode := Run([]string{"reset", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := os.Getenv("ESB_MODE"); got != "firecracker" {
		t.Fatalf("unexpected ESB_MODE: %s", got)
	}
}

func TestRunResetUsesActiveEnvFromGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv("ESB_MODE", "")
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_ENV", "staging")

	downer := &fakeResetDowner{}
	builder := &fakeResetBuilder{}
	upper := &fakeResetUpper{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Downer:     downer,
		Builder:    builder,
		Upper:      upper,
	}

	exitCode := Run([]string{"reset", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
	if len(builder.requests) != 1 || builder.requests[0].Env != "staging" {
		t.Fatalf("unexpected build env: %v", builder.requests)
	}
	if len(upper.requests) != 1 || upper.requests[0].Context.Env != "staging" {
		t.Fatalf("unexpected up env: %v", upper.requests)
	}
}
