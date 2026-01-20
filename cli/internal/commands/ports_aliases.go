// Where: cli/internal/commands/ports_aliases.go
// What: Aliases for port request types used in command tests.
// Why: Keep tests readable while the commands layer depends on ports.
package commands

import "github.com/poruru/edge-serverless-box/cli/internal/ports"

type (
	LogsRequest   = ports.LogsRequest
	PruneRequest  = ports.PruneRequest
	StateDetector = ports.StateDetector
	StopRequest   = ports.StopRequest
	UpRequest     = ports.UpRequest
)
