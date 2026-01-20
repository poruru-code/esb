// Where: cli/internal/app/ports_aliases.go
// What: Aliases from app package to ports definitions.
// Why: Keep existing references to app-specific interfaces while sharing the centralized ports definitions.
package app

import "github.com/poruru/edge-serverless-box/cli/internal/ports"

type (
	Downer            = ports.Downer
	Upper             = ports.Upper
	GatewayWaiter     = ports.GatewayWaiter
	StateDetector     = ports.StateDetector
	DetectorFactory   = ports.DetectorFactory
	CredentialManager = ports.CredentialManager
	TemplateLoader    = ports.TemplateLoader
	TemplateParser    = ports.TemplateParser
	PortPublisher     = ports.PortPublisher
	UpRequest         = ports.UpRequest
	Logger            = ports.Logger
	LogsRequest       = ports.LogsRequest
	Stopper           = ports.Stopper
	StopRequest       = ports.StopRequest
	Pruner            = ports.Pruner
	PruneRequest      = ports.PruneRequest
)
