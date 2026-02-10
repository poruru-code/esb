// Where: cli/internal/infra/build/go_builder_phase.go
// What: Build phase progress reporting helpers.
// Why: Keep user-facing phase output separate from build orchestration logic.
package build

import (
	"fmt"
	"time"
)

type phaseReporter struct {
	verbose bool
	emoji   bool
}

func newPhaseReporter(verbose, emoji bool) phaseReporter {
	return phaseReporter{verbose: verbose, emoji: emoji}
}

func (p phaseReporter) Run(label string, fn func() error) error {
	start := time.Now()
	err := fn()
	if p.verbose {
		return err
	}
	duration := time.Since(start)
	ok := err == nil
	status := "ok"
	if !ok {
		status = "failed"
	}
	prefix := p.prefix(ok)
	fmt.Printf("%s%s ... %s (%s)\n", prefix, label, status, formatDuration(duration))
	return err
}

func (p phaseReporter) prefix(ok bool) string {
	if p.emoji {
		if ok {
			return "✅ "
		}
		return "❌ "
	}
	if ok {
		return "[ok] "
	}
	return "[fail] "
}

func formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%.1fs", duration.Seconds())
	}
	total := int(duration.Seconds())
	mins := total / 60
	secs := total % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}
