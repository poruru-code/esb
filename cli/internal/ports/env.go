// Where: cli/internal/ports/env.go
// What: Runtime environment applier port.
// Why: Workflows need to set runtime-specific environment variables through a stable interface.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// RuntimeEnvApplier applies runtime defaults such as env vars to the current process.
type RuntimeEnvApplier interface {
	Apply(ctx state.Context)
}
