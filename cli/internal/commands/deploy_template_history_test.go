// Where: cli/internal/commands/deploy_template_history_test.go
// What: Tests for template history management helpers.
// Why: Ensure recent template ordering and limits are stable across deploy runs.
package commands

import (
	"fmt"
	"testing"
)

func TestUpdateTemplateHistoryMovesRecentToFront(t *testing.T) {
	history := []string{"/tmp/a.yaml", "/tmp/b.yaml", "/tmp/c.yaml"}

	got := updateTemplateHistory(history, "/tmp/b.yaml")
	want := []string{"/tmp/b.yaml", "/tmp/a.yaml", "/tmp/c.yaml"}

	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected entry at %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUpdateTemplateHistoryLimitsEntries(t *testing.T) {
	history := make([]string, 0, templateHistoryLimit+2)
	for i := 0; i < templateHistoryLimit+2; i++ {
		history = append(history, fmt.Sprintf("/tmp/template-%d.yaml", i))
	}

	got := updateTemplateHistory(history, "/tmp/new.yaml")
	if len(got) != templateHistoryLimit {
		t.Fatalf("unexpected length: got %d, want %d", len(got), templateHistoryLimit)
	}
	if got[0] != "/tmp/new.yaml" {
		t.Fatalf("expected newest entry first, got %q", got[0])
	}
}
