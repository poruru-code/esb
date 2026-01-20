// Where: cli/internal/workflows/stop.go
// What: Stop workflow orchestration.
// Why: Keep CLI adapter minimal while preserving stop behavior.
package workflows

import (
	"errors"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// StopRequest captures inputs for stopping an environment.
type StopRequest struct {
	Context state.Context
}

// StopWorkflow orchestrates a stop request.
type StopWorkflow struct {
	Stopper       ports.Stopper
	EnvApplier    ports.RuntimeEnvApplier
	UserInterface ports.UserInterface
}

// NewStopWorkflow constructs a StopWorkflow.
func NewStopWorkflow(stopper ports.Stopper, envApplier ports.RuntimeEnvApplier, ui ports.UserInterface) StopWorkflow {
	return StopWorkflow{
		Stopper:       stopper,
		EnvApplier:    envApplier,
		UserInterface: ui,
	}
}

// Run executes the workflow.
func (w StopWorkflow) Run(req StopRequest) error {
	if w.EnvApplier != nil {
		w.EnvApplier.Apply(req.Context)
	}
	if w.Stopper == nil {
		return errors.New("stopper not configured")
	}
	if err := w.Stopper.Stop(ports.StopRequest{Context: req.Context}); err != nil {
		return err
	}
	if w.UserInterface != nil {
		w.UserInterface.Success("stop complete")
	}
	return nil
}
