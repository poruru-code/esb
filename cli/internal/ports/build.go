// Where: cli/internal/ports/build.go
// What: Builder port interface for workflows.
// Why: Allow workflows to call into generation/building implementations via well-defined contracts.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/generator"

// Builder builds the Docker artifacts for Lambda functions.
type Builder interface {
	Build(request generator.BuildRequest) error
}
