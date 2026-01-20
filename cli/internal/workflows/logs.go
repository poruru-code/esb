// Where: cli/internal/workflows/logs.go
// What: Logs workflow orchestration.
// Why: Keep CLI logic focused on input/prompts while the workflow executes streaming.
package workflows

import (
	"errors"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

// LogsRequest mirrors ports.LogsRequest but allows extension in this layer if needed.
type LogsRequest struct {
	ports.LogsRequest
}

// LogsWorkflow executes logging workflows against the provided logger port.
type LogsWorkflow struct {
	Logger        ports.Logger
	EnvApplier    ports.RuntimeEnvApplier
	UserInterface ports.UserInterface
}

// NewLogsWorkflow constructs a LogsWorkflow.
func NewLogsWorkflow(logger ports.Logger, envApplier ports.RuntimeEnvApplier, ui ports.UserInterface) LogsWorkflow {
	return LogsWorkflow{
		Logger:        logger,
		EnvApplier:    envApplier,
		UserInterface: ui,
	}
}

// Run executes the workflow and returns any logger errors.
func (w LogsWorkflow) Run(req LogsRequest) error {
	if w.Logger == nil {
		return errors.New("logger not configured")
	}
	if w.EnvApplier != nil {
		w.EnvApplier.Apply(req.Context)
	}
	return w.Logger.Logs(req.LogsRequest)
}
