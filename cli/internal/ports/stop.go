// Where: cli/internal/ports/stop.go
// What: Stopper port definitions.
// Why: Allow workflows to stop environments via a stable interface.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// StopRequest contains parameters for stopping the environment.
// Unlike Down, this preserves container state for later restart.
type StopRequest struct {
	Context state.Context
}

// Stopper defines the interface for stopping containers without removal.
type Stopper interface {
	Stop(request StopRequest) error
}
