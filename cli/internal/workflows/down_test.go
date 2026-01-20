// Where: cli/internal/workflows/down_test.go
// What: Unit tests for DownWorkflow orchestration.
// Why: Ensure down requests are delegated to the Downer port.
package workflows

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestDownWorkflowRunSuccess(t *testing.T) {
	downer := &recordDowner{}
	ui := &testUI{}

	workflow := NewDownWorkflow(downer, ui)
	ctx := state.Context{ComposeProject: "esb-dev"}
	if err := workflow.Run(DownRequest{Context: ctx, Volumes: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(downer.calls) != 1 || downer.calls[0].project != "esb-dev" || !downer.calls[0].volumes {
		t.Fatalf("expected downer to be called with volumes")
	}
	if len(ui.successes) != 1 || !strings.Contains(ui.successes[0], "down complete") {
		t.Fatalf("expected success message")
	}
}

func TestDownWorkflowMissingDowner(t *testing.T) {
	workflow := NewDownWorkflow(nil, nil)
	err := workflow.Run(DownRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "downer not configured") {
		t.Fatalf("expected downer missing error, got %v", err)
	}
}
