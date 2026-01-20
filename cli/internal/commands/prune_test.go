// Where: cli/internal/commands/prune_test.go
// What: Tests for prune command wiring.
// Why: Ensure prune removes artifacts with confirmation.
package commands

import (
	"bytes"
	"os"
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

func TestRunPruneCallsPruner(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
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
	if req.RemoveVolumes {
		t.Fatalf("expected remove volumes false by default")
	}
	if req.AllImages {
		t.Fatalf("expected all images false by default")
	}
}

func TestRunPruneWithHard(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes", "--hard"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(pruner.requests) != 1 || !pruner.requests[0].Hard {
		t.Fatalf("expected hard prune request, got %v", pruner.requests)
	}
}

func TestRunPruneWithVolumesAndAllImages(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes", "--volumes", "--all"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(pruner.requests) != 1 {
		t.Fatalf("expected prune called once, got %d", len(pruner.requests))
	}
	req := pruner.requests[0]
	if !req.RemoveVolumes {
		t.Fatalf("expected remove volumes true")
	}
	if !req.AllImages {
		t.Fatalf("expected all images true")
	}
}

func TestRunPruneRequiresYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	pruner := &fakePruner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	originalIsTerminal := isTerminal
	isTerminal = func(_ *os.File) bool { return false }
	defer func() { isTerminal = originalIsTerminal }()

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

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing pruner")
	}
}

func TestRunPruneWithEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	pruner := &fakePruner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"--env", "staging", "prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
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
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
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
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
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
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Prune: PruneDeps{Pruner: pruner}}

	exitCode := Run([]string{"prune", "--yes"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without generator.yml")
	}
	if len(pruner.requests) != 0 {
		t.Fatalf("expected no prune calls when generator.yml missing")
	}
}
