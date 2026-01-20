// Where: cli/internal/commands/deps_split.go
// What: Narrow dependency bundles for workflow-backed commands.
// Why: Reduce the surface area of Dependencies by grouping command wiring.
package commands

import (
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

// BuildDeps holds only the dependencies required by the build command.
type BuildDeps struct {
	Builder ports.Builder
}

// UpDeps holds only the dependencies required by the up command.
type UpDeps struct {
	Builder        ports.Builder
	Upper          ports.Upper
	Downer         ports.Downer
	PortDiscoverer helpers.PortDiscoverer
	Waiter         ports.GatewayWaiter
	Provisioner    ports.Provisioner
	Parser         generator.Parser
}

// DownDeps holds only the dependencies required by the down command.
type DownDeps struct {
	Downer ports.Downer
}

// LogsDeps holds only the dependencies required by the logs command.
type LogsDeps struct {
	Logger ports.Logger
}

// StopDeps holds only the dependencies required by the stop command.
type StopDeps struct {
	Stopper ports.Stopper
}

// PruneDeps holds only the dependencies required by the prune command.
type PruneDeps struct {
	Pruner ports.Pruner
}
