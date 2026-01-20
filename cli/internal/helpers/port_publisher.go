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
	store      ports.StateStore
}

func NewPortPublisher(discoverer PortDiscoverer, store ports.StateStore) ports.PortPublisher {
	return portPublisher{discoverer: discoverer, store: store}
}

func (p portPublisher) Publish(ctx state.Context) (ports.PortPublishResult, error) {
	return DiscoverAndPersistPorts(ctx, p.discoverer, p.store)
}
