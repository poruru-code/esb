// Where: cli/internal/workflows/logs_test.go
// What: Unit tests for LogsWorkflow orchestration.
// Why: Verify logger invocation and env application without CLI adapters.
package workflows

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestLogsWorkflowRunSuccess(t *testing.T) {
	logger := &recordLogger{}
	envApplier := &recordEnvApplier{}

	workflow := NewLogsWorkflow(logger, envApplier, nil)
	req := LogsRequest{
		LogsRequest: ports.LogsRequest{
			Context: state.Context{ComposeProject: "esb-dev"},
			Follow:  true,
			Tail:    10,
		},
	}
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.calls) != 1 {
		t.Fatalf("expected env applier to be called once")
	}
	if len(logger.requests) != 1 || logger.requests[0].Tail != 10 || !logger.requests[0].Follow {
		t.Fatalf("expected logger to be called with request")
	}
}

func TestLogsWorkflowMissingLogger(t *testing.T) {
	workflow := NewLogsWorkflow(nil, nil, nil)
	err := workflow.Run(LogsRequest{LogsRequest: ports.LogsRequest{Context: state.Context{}}})
	if err == nil || !strings.Contains(err.Error(), "logger not configured") {
		t.Fatalf("expected logger missing error, got %v", err)
	}
}
