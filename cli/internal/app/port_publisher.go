// Where: cli/internal/app/port_publisher.go
// What: PortPublisher adapter for workflows.
// Why: Allow workflows to discover and persist ports via a port-friendly interface.
package app

import (
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type portPublisher struct {
	discoverer PortDiscoverer
}

func newPortPublisher(discoverer PortDiscoverer) ports.PortPublisher {
	return portPublisher{discoverer: discoverer}
}

func (p portPublisher) Publish(ctx state.Context) (map[string]int, error) {
	return DiscoverAndPersistPorts(ctx, p.discoverer)
}
