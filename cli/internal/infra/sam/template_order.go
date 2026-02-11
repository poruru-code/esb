// Where: cli/internal/infra/sam/template_order.go
// What: Deterministic key-order helpers for map-backed template sections.
// Why: Keep parser output stable across Go map iteration orders.
package sam

import "sort"

func sortedMapKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
