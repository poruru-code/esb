// Where: cli/internal/infra/sam/intrinsics_warnings.go
// What: Warning aggregation helpers for intrinsic resolver.
// Why: Keep warning deduplication logic isolated and reusable across resolver phases.
package sam

import "fmt"

func (r *IntrinsicResolver) addWarning(msg string) {
	if _, seen := r.warningsSeen[msg]; seen {
		return
	}
	r.Warnings = append(r.Warnings, msg)
	r.warningsSeen[msg] = struct{}{}
}

func (r *IntrinsicResolver) addWarningf(format string, args ...any) {
	r.addWarning(fmt.Sprintf(format, args...))
}
