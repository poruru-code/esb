// Where: cli/internal/workflows/down.go
// What: Down workflow orchestration.
// Why: Keep CLI adapter minimal and reuse the ports interface for down logic.
package workflows

import (
	"errors"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// DownRequest captures inputs for tearing down an environment.
type DownRequest struct {
	Context state.Context
	Volumes bool
}

// DownWorkflow orchestrates a down request.
type DownWorkflow struct {
	Downer        ports.Downer
	UserInterface ports.UserInterface
}

// NewDownWorkflow constructs a DownWorkflow.
func NewDownWorkflow(downer ports.Downer, ui ports.UserInterface) DownWorkflow {
	return DownWorkflow{
		Downer:        downer,
		UserInterface: ui,
	}
}

// Run executes the workflow.
func (w DownWorkflow) Run(req DownRequest) error {
	if w.Downer == nil {
		return errors.New("downer not configured")
	}
	if err := w.Downer.Down(req.Context.ComposeProject, req.Volumes); err != nil {
		return err
	}
	if w.UserInterface != nil {
		w.UserInterface.Success("down complete")
	}
	return nil
}
