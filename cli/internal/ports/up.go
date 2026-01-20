// Where: cli/internal/ports/up.go
// What: Ports needed by the Up workflow.
// Why: Provide stable contracts between CLI and orchestration logic.
package ports

import (
	"context"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// UpRequest contains the parameters needed to run docker compose up.
type UpRequest struct {
	Context state.Context
	Detach  bool
	Wait    bool
	EnvFile string
}

// Upper brings up the control-plane services via Docker Compose.
type Upper interface {
	Up(request UpRequest) error
}

// Downer tears down the control-plane services via Docker Compose.
type Downer interface {
	Down(project string, removeVolumes bool) error
}

// PortPublisher discovers runtime ports, persists them, and exposes them as env vars.
type PortPublisher interface {
	Publish(ctx state.Context) (map[string]int, error)
}

// TemplateLoader reads the SAM template file.
type TemplateLoader interface {
	Read(path string) (string, error)
}

// TemplateParser parses SAM templates into the manifest spec.
type TemplateParser interface {
	Parse(content string, parameters map[string]string) (generator.ParseResult, error)
}

// Provisioner applies resources to the runtime environment.
type Provisioner interface {
	Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error
}

// GatewayWaiter waits until the gateway reports readiness.
type GatewayWaiter interface {
	Wait(ctx state.Context) error
}
