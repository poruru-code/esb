// Where: cli/internal/ports/prune.go
// What: Pruner port definitions.
// Why: Allow workflows to prune resources via a stable interface.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// PruneRequest contains parameters for removing project resources and artifacts.
// The Hard flag also removes the generator.yml configuration file.
type PruneRequest struct {
	Context       state.Context
	Hard          bool
	RemoveVolumes bool
	AllImages     bool
}

// Pruner defines the interface for removing project resources and artifacts.
type Pruner interface {
	Prune(request PruneRequest) error
}
