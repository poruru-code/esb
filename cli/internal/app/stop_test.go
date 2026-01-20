// Where: cli/internal/app/stop_test.go
// What: Tests for stop/logs command wiring.
// Why: Ensure stop/logs invoke compose helpers with resolved context.
package app

import (
	"bytes"
	"testing"
)

type fakeStopper struct {
	requests []StopRequest
	err      error
}

func (f *fakeStopper) Stop(request StopRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

func TestRunStopCallsStopper(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	stopper := &fakeStopper{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Stop: StopDeps{Stopper: stopper}}

	exitCode := Run([]string{"stop"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(stopper.requests) != 1 {
		t.Fatalf("expected stop called once, got %d", len(stopper.requests))
	}
	if stopper.requests[0].Context.ComposeProject != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected project: %s", stopper.requests[0].Context.ComposeProject)
	}
}

func TestRunStopMissingStopper(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"stop"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing stopper")
	}
}
