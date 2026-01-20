// Where: cli/internal/app/deps_split.go
// What: Narrow dependency bundles for workflow-backed commands.
// Why: Reduce the surface area of Dependencies by grouping command wiring.
package app

import "github.com/poruru/edge-serverless-box/cli/internal/generator"

// BuildDeps holds only the dependencies required by the build command.
type BuildDeps struct {
	Builder Builder
}

// UpDeps holds only the dependencies required by the up command.
type UpDeps struct {
	Builder        Builder
	Upper          Upper
	Downer         Downer
	PortDiscoverer PortDiscoverer
	Waiter         GatewayWaiter
	Provisioner    Provisioner
	Parser         generator.Parser
}

// DownDeps holds only the dependencies required by the down command.
type DownDeps struct {
	Downer Downer
}

// LogsDeps holds only the dependencies required by the logs command.
type LogsDeps struct {
	Logger Logger
}

// StopDeps holds only the dependencies required by the stop command.
type StopDeps struct {
	Stopper Stopper
}

// PruneDeps holds only the dependencies required by the prune command.
type PruneDeps struct {
	Pruner Pruner
}
