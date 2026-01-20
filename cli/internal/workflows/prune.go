// Where: cli/internal/workflows/prune.go
// What: Prune workflow orchestration.
// Why: Keep CLI adapter minimal while preserving prune behavior.
package workflows

import (
	"errors"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// PruneRequest captures inputs for pruning resources and artifacts.
type PruneRequest struct {
	Context       state.Context
	Hard          bool
	RemoveVolumes bool
	AllImages     bool
}

// PruneWorkflow orchestrates a prune request.
type PruneWorkflow struct {
	Pruner        ports.Pruner
	UserInterface ports.UserInterface
}

// NewPruneWorkflow constructs a PruneWorkflow.
func NewPruneWorkflow(pruner ports.Pruner, ui ports.UserInterface) PruneWorkflow {
	return PruneWorkflow{
		Pruner:        pruner,
		UserInterface: ui,
	}
}

// Run executes the workflow.
func (w PruneWorkflow) Run(req PruneRequest) error {
	if w.Pruner == nil {
		return errors.New("pruner not configured")
	}
	if err := w.Pruner.Prune(ports.PruneRequest{
		Context:       req.Context,
		Hard:          req.Hard,
		RemoveVolumes: req.RemoveVolumes,
		AllImages:     req.AllImages,
	}); err != nil {
		return err
	}
	if w.UserInterface != nil {
		w.UserInterface.Success("prune complete")
	}
	return nil
}
