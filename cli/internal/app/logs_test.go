package app

import (
	"bytes"
	"testing"
)

type fakeLogger struct {
	requests     []LogsRequest
	listRequests []LogsRequest
	services     []string
	err          error
	listErr      error
}

func (f *fakeLogger) Logs(request LogsRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

func (f *fakeLogger) ListServices(request LogsRequest) ([]string, error) {
	f.listRequests = append(f.listRequests, request)
	return f.services, f.listErr
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

	// Non-interactive execution with arguments
	exitCode := Run([]string{"logs", "--follow", "--tail", "50", "--timestamps", "gateway"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(logger.requests) != 1 {
		t.Fatalf("expected logs called once, got %d", len(logger.requests))
	}
	req := logger.requests[0]
	if req.Context.ComposeProject != expectedComposeProject("demo", "default") {
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
