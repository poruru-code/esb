// Where: cli/internal/ports/detector.go
// What: Environment state detector ports.
// Why: Share detector interfaces between app and workflows.
package ports

import "github.com/poruru/edge-serverless-box/cli/internal/state"

// StateDetector reports the current environment state (running/stopped/absent).
type StateDetector interface {
	Detect() (state.State, error)
}

// DetectorFactory builds a StateDetector for a project/environment pair.
type DetectorFactory func(projectDir, env string) (StateDetector, error)
