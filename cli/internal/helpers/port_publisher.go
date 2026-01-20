// Where: cli/internal/helpers/port_publisher.go
// What: PortPublisher adapter for workflows.
// Why: Allow workflows to discover and persist ports via a port-friendly interface.
package helpers

import (
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type portPublisher struct {
	discoverer PortDiscoverer
}

func NewPortPublisher(discoverer PortDiscoverer) ports.PortPublisher {
	return portPublisher{discoverer: discoverer}
}

func (p portPublisher) Publish(ctx state.Context) (map[string]int, error) {
	return DiscoverAndPersistPorts(ctx, p.discoverer)
}
