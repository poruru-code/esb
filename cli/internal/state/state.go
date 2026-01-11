// Where: cli/internal/state/state.go
// What: State definitions and derivation helpers.
// Why: Centralize state transitions for the CLI detector.
package state

// State represents the current CLI environment state.
type State string

const (
	StateUninitialized State = "uninitialized"
	StateInitialized   State = "initialized"
	StateBuilt         State = "built"
	StateRunning       State = "running"
	StateStopped       State = "stopped"
)

// ContainerInfo holds information about a Docker container.
type ContainerInfo struct {
	Name    string
	Service string
	State   string
}

// DeriveState determines the environment state based on context validity,
// container presence, and build artifacts.
func DeriveState(contextValid bool, containers []ContainerInfo, hasArtifacts bool) State {
	if !contextValid {
		return StateUninitialized
	}

	if countRunning(containers) > 0 {
		return StateRunning
	}
	if len(containers) > 0 {
		return StateStopped
	}
	if hasArtifacts {
		return StateBuilt
	}
	return StateInitialized
}

// countRunning counts how many containers are in the "running" state.
func countRunning(containers []ContainerInfo) int {
	count := 0
	for _, container := range containers {
		if container.State == "running" {
			count++
		}
	}
	return count
}
