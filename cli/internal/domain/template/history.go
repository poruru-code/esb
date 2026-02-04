// Where: cli/internal/domain/template/history.go
// What: Pure helpers for template history and suggestions.
// Why: Keep history logic deterministic and independent from I/O.
package template

import "strings"

// BuildSuggestions merges previous path, history, and candidates into a unique list.
func BuildSuggestions(previous string, history, candidates []string) []string {
	suggestions := []string{}
	seen := map[string]struct{}{}
	add := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		suggestions = append(suggestions, trimmed)
		seen[trimmed] = struct{}{}
	}

	add(previous)
	for _, entry := range history {
		add(entry)
	}
	for _, candidate := range candidates {
		add(candidate)
	}
	return suggestions
}

// UpdateHistory inserts templatePath at the front and enforces a limit.
func UpdateHistory(history []string, templatePath string, limit int) []string {
	trimmed := strings.TrimSpace(templatePath)
	if trimmed == "" {
		return history
	}
	next := make([]string, 0, limit)
	seen := map[string]struct{}{}
	add := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		if limit > 0 && len(next) >= limit {
			return
		}
		next = append(next, trimmed)
		seen[trimmed] = struct{}{}
	}

	add(trimmed)
	for _, entry := range history {
		add(entry)
	}
	return next
}
