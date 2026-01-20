// Where: cli/internal/workflows/prune_test.go
// What: Unit tests for PruneWorkflow orchestration.
// Why: Verify pruner invocation and success reporting.
package workflows

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestPruneWorkflowRunSuccess(t *testing.T) {
	pruner := &recordPruner{}
	ui := &testUI{}

	workflow := NewPruneWorkflow(pruner, ui)
	ctx := state.Context{ComposeProject: "esb-dev"}
	err := workflow.Run(PruneRequest{Context: ctx, Hard: true, RemoveVolumes: true, AllImages: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(pruner.requests) != 1 {
		t.Fatalf("expected pruner to be called once")
	}
	req := pruner.requests[0]
	if !req.Hard || !req.RemoveVolumes || !req.AllImages {
		t.Fatalf("expected prune flags to be set")
	}
	if len(ui.successes) != 1 || !strings.Contains(ui.successes[0], "prune complete") {
		t.Fatalf("expected success message")
	}
}

func TestPruneWorkflowMissingPruner(t *testing.T) {
	workflow := NewPruneWorkflow(nil, nil)
	err := workflow.Run(PruneRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "pruner not configured") {
		t.Fatalf("expected pruner missing error, got %v", err)
	}
}
