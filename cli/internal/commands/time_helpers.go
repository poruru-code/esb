// Where: cli/internal/commands/time_helpers.go
// What: Helpers for deterministic current time usage.
// Why: Allow tests to override the clock via Dependencies.
package commands

import "time"

func now(deps Dependencies) time.Time {
	if deps.Now != nil {
		return deps.Now()
	}
	return time.Now()
}
