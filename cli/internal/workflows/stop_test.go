// Where: cli/internal/workflows/stop_test.go
// What: Unit tests for StopWorkflow orchestration.
// Why: Ensure stop requests are delegated to the Stopper port.
package workflows

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestStopWorkflowRunSuccess(t *testing.T) {
	stopper := &recordStopper{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}

	workflow := NewStopWorkflow(stopper, envApplier, ui)
	ctx := state.Context{ComposeProject: "esb-dev"}
	if err := workflow.Run(StopRequest{Context: ctx}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.calls) != 1 {
		t.Fatalf("expected env applier to be called once")
	}
	if len(stopper.requests) != 1 || stopper.requests[0].Context.ComposeProject != "esb-dev" {
		t.Fatalf("expected stopper to be called")
	}
	if len(ui.successes) != 1 || !strings.Contains(ui.successes[0], "stop complete") {
		t.Fatalf("expected success message")
	}
}

func TestStopWorkflowMissingStopper(t *testing.T) {
	workflow := NewStopWorkflow(nil, nil, nil)
	err := workflow.Run(StopRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "stopper not configured") {
		t.Fatalf("expected stopper missing error, got %v", err)
	}
}
