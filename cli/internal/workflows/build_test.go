// Where: cli/internal/workflows/build_test.go
// What: Unit tests for BuildWorkflow orchestration.
// Why: Verify builder wiring and runtime env application without CLI adapters.
package workflows

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestBuildWorkflowRunSuccess(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}

	ctx := state.Context{
		ProjectDir:     "/repo",
		ComposeProject: "esb-dev",
	}
	req := BuildRequest{
		Context:      ctx,
		Env:          "dev",
		TemplatePath: "/repo/template.yaml",
		NoCache:      true,
		Verbose:      true,
	}

	workflow := NewBuildWorkflow(builder, envApplier, ui)
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.calls) != 1 {
		t.Fatalf("expected env applier to be called once, got %d", len(envApplier.calls))
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called once, got %d", len(builder.requests))
	}

	got := builder.requests[0]
	if got.ProjectDir != req.Context.ProjectDir {
		t.Fatalf("project dir mismatch: %s", got.ProjectDir)
	}
	if got.ProjectName != req.Context.ComposeProject {
		t.Fatalf("project name mismatch: %s", got.ProjectName)
	}
	if got.TemplatePath != req.TemplatePath {
		t.Fatalf("template path mismatch: %s", got.TemplatePath)
	}
	if got.Env != req.Env {
		t.Fatalf("env mismatch: %s", got.Env)
	}
	if !got.NoCache || !got.Verbose {
		t.Fatalf("expected no-cache and verbose to be true")
	}

	if len(ui.successes) != 1 || !strings.Contains(ui.successes[0], "Build complete") {
		t.Fatalf("expected build success message")
	}
	if len(ui.infos) != 1 || ui.infos[0] != "Next: esb up" {
		t.Fatalf("expected next step info")
	}
}

func TestBuildWorkflowRunMissingBuilder(t *testing.T) {
	workflow := NewBuildWorkflow(nil, nil, nil)
	err := workflow.Run(BuildRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "builder port is not configured") {
		t.Fatalf("expected builder missing error, got %v", err)
	}
}
