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
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Stopper: stopper}

	exitCode := Run([]string{"stop"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(stopper.requests) != 1 {
		t.Fatalf("expected stop called once, got %d", len(stopper.requests))
	}
	if stopper.requests[0].Context.ComposeProject != "esb-default" {
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

type fakeLogger struct {
	requests []LogsRequest
	err      error
}

func (f *fakeLogger) Logs(request LogsRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

func TestRunLogsCallsLogger(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	logger := &fakeLogger{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Logger: logger}

	exitCode := Run([]string{"logs", "--follow", "--tail", "50", "--timestamps", "gateway"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(logger.requests) != 1 {
		t.Fatalf("expected logs called once, got %d", len(logger.requests))
	}
	req := logger.requests[0]
	if req.Context.ComposeProject != "esb-default" {
		t.Fatalf("unexpected project: %s", req.Context.ComposeProject)
	}
	if !req.Follow || !req.Timestamps || req.Tail != 50 || req.Service != "gateway" {
		t.Fatalf("unexpected logs request: %+v", req)
	}
}

func TestRunLogsMissingLogger(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"logs"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing logger")
	}
}
