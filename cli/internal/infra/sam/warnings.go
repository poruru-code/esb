// Where: cli/internal/infra/sam/warnings.go
// What: Warning aggregation helpers for template parsing.
// Why: Avoid parser-side direct stdout writes while preserving diagnostics.
package sam

import "fmt"

type warningCollector struct {
	warnings []string
}

func (c *warningCollector) warnf(format string, args ...any) {
	if c == nil {
		return
	}
	c.warnings = append(c.warnings, fmt.Sprintf(format, args...))
}

func (c *warningCollector) list() []string {
	if c == nil || len(c.warnings) == 0 {
		return nil
	}
	out := make([]string, len(c.warnings))
	copy(out, c.warnings)
	return out
}
