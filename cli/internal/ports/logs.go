// Where: cli/internal/ports/logs.go
// What: Logger port definitions.
// Why: Allow workflows to consume log streaming capabilities via interfaces.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// LogsRequest captures parameters for streaming logs.
type LogsRequest struct {
	Context    state.Context
	Follow     bool
	Tail       int
	Timestamps bool
	Service    string
}

// Logger streams Docker Compose logs and lists services/containers.
type Logger interface {
	Logs(request LogsRequest) error
	ListServices(request LogsRequest) ([]string, error)
	ListContainers(project string) ([]state.ContainerInfo, error)
}
