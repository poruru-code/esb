// Where: cli/internal/ports/state_store.go
// What: Port state persistence contracts.
// Why: Allow port discovery results to be stored and retrieved consistently.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// StateStore persists discovered port mappings for an environment.
type StateStore interface {
	Load(ctx state.Context) (map[string]int, error)
	Save(ctx state.Context, ports map[string]int) error
	Remove(ctx state.Context) error
}
