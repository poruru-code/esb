// Where: cli/internal/app/prune_test.go
// What: Tests for prune command wiring.
// Why: Ensure prune removes artifacts with confirmation.
package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type fakePruner struct {
	requests []PruneRequest
	err      error
}

func (f *fakePruner) Prune(request PruneRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

type fakePruneDowner struct {
	projects      []string
	removeVolumes []bool
	err           error
}

func (f *fakePruneDowner) Down(project string, removeVolumes bool) error {
	f.projects = append(f.projects, project)
	f.removeVolumes = append(f.removeVolumes, removeVolumes)
	return f.err
}

func TestRunPruneCallsPruner(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected down project: %v", downer.projects)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
	if len(pruner.requests) != 1 {
		t.Fatalf("expected prune called once, got %d", len(pruner.requests))
	}
	req := pruner.requests[0]
	if req.Context.ComposeProject != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected compose project: %s", req.Context.ComposeProject)
	}
	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if req.Context.TemplatePath != expectedTemplate {
		t.Fatalf("unexpected template path: %s", req.Context.TemplatePath)
	}
	if req.Hard {
		t.Fatalf("expected hard prune false by default")
	}
}

func TestRunPruneWithHard(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune", "--yes", "--hard"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
	if len(pruner.requests) != 1 || !pruner.requests[0].Hard {
		t.Fatalf("expected hard prune request, got %v", pruner.requests)
	}
}

func TestRunPruneRequiresYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without --yes")
	}
	if len(pruner.requests) != 0 {
		t.Fatalf("expected no prune calls without confirmation")
	}
}

func TestRunPruneMissingPruner(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Downer: downer}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing pruner")
	}
	if len(downer.projects) != 1 {
		t.Fatalf("expected downer to be called")
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
}

func TestRunPruneWithEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"--env", "staging", "prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
	if len(pruner.requests) != 1 || pruner.requests[0].Context.ComposeProject != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected context: %v", pruner.requests)
	}
}

func TestRunPrunePassesContext(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
	if len(pruner.requests) != 1 {
		t.Fatalf("expected prune called once, got %d", len(pruner.requests))
	}
	if pruner.requests[0].Context.Env != "default" {
		t.Fatalf("unexpected env: %s", pruner.requests[0].Context.Env)
	}
	if pruner.requests[0].Context.ProjectDir == "" {
		t.Fatalf("expected project dir in context")
	}
	if pruner.requests[0].Context.OutputEnvDir == "" {
		t.Fatalf("expected output env dir in context")
	}
}

func TestRunPruneUsesActiveEnvFromGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_ENV", "staging")

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected volumes removed")
	}
	if len(pruner.requests) != 1 {
		t.Fatalf("expected prune called once, got %d", len(pruner.requests))
	}
	if pruner.requests[0].Context.Env != "staging" {
		t.Fatalf("unexpected env: %s", pruner.requests[0].Context.Env)
	}
	if pruner.requests[0].Context.ComposeProject != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected compose project: %s", pruner.requests[0].Context.ComposeProject)
	}
}

func TestRunPruneWithoutGeneratorRemovesContainersOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()

	pruner := &fakePruner{}
	downer := &fakePruneDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Pruner: pruner, Downer: downer}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without generator.yml")
	}
	if len(downer.projects) != 0 {
		t.Fatalf("expected no down calls when generator.yml missing")
	}
	if len(pruner.requests) != 0 {
		t.Fatalf("expected no prune calls when generator.yml missing")
	}
}
