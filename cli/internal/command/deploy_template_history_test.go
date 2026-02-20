// Where: cli/internal/command/deploy_template_history_test.go
// What: Tests for template history management env.
// Why: Ensure recent template ordering and limits are stable across deploy runs.
package command

import (
	"fmt"
	"testing"

	domaintpl "github.com/poruru-code/esb/cli/internal/domain/template"
)

func TestUpdateTemplateHistoryMovesRecentToFront(t *testing.T) {
	history := []string{"/tmp/a.yaml", "/tmp/b.yaml", "/tmp/c.yaml"}

	got := domaintpl.UpdateHistory(history, "/tmp/b.yaml", templateHistoryLimit)
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
	for i := range templateHistoryLimit + 2 {
		history = append(history, fmt.Sprintf("/tmp/template-%d.yaml", i))
	}

	got := domaintpl.UpdateHistory(history, "/tmp/new.yaml", templateHistoryLimit)
	if len(got) != templateHistoryLimit {
		t.Fatalf("unexpected length: got %d, want %d", len(got), templateHistoryLimit)
	}
	if got[0] != "/tmp/new.yaml" {
		t.Fatalf("expected newest entry first, got %q", got[0])
	}
}
