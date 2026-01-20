// Where: cli/internal/helpers/runtime_env.go
// What: Runtime environment applier adapter.
// Why: Expose applyRuntimeEnv via ports.RuntimeEnvApplier.
package helpers

import (
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type runtimeEnvApplier struct {
	resolver func(string) (string, error)
}

func NewRuntimeEnvApplier(resolver func(string) (string, error)) ports.RuntimeEnvApplier {
	if resolver == nil {
		resolver = config.ResolveRepoRoot
	}
	return runtimeEnvApplier{resolver: resolver}
}

func (r runtimeEnvApplier) Apply(ctx state.Context) {
	applyRuntimeEnv(ctx, r.resolver)
}
