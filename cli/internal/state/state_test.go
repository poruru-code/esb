// Where: cli/internal/state/state_test.go
// What: Tests for state derivation logic.
// Why: Keep detection rules deterministic and easy to validate.
package state

import "testing"

func TestDeriveState(t *testing.T) {
	t.Run("uninitialized when context invalid", func(t *testing.T) {
		state := DeriveState(false, nil, false)
		assertState(t, StateUninitialized, state)
	})

	t.Run("running when any container running", func(t *testing.T) {
		containers := []ContainerInfo{{State: "running"}, {State: "exited"}}
		state := DeriveState(true, containers, true)
		assertState(t, StateRunning, state)
	})

	t.Run("stopped when containers exist but none running", func(t *testing.T) {
		containers := []ContainerInfo{{State: "exited"}, {State: "created"}}
		state := DeriveState(true, containers, true)
		assertState(t, StateStopped, state)
	})

	t.Run("built when artifacts exist and no containers", func(t *testing.T) {
		state := DeriveState(true, nil, true)
		assertState(t, StateBuilt, state)
	})

	t.Run("initialized when no artifacts and no containers", func(t *testing.T) {
		state := DeriveState(true, nil, false)
		assertState(t, StateInitialized, state)
	})
}

func assertState(t *testing.T, expected, actual State) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected state %s, got %s", expected, actual)
	}
}
