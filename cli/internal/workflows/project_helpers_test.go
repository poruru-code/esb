// Where: cli/internal/workflows/project_helpers_test.go
// What: Unit tests for project helper functions.
// Why: Ensure project selection and sorting logic is deterministic.
package workflows

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestSortProjectsByRecentOrdersByTimeAndName(t *testing.T) {
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"bravo":   {LastUsed: "2025-01-02T00:00:00Z"},
			"alpha":   {LastUsed: "2025-01-02T00:00:00Z"},
			"charlie": {},
		},
	}

	list := SortProjectsByRecent(cfg)
	if len(list) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "bravo" {
		t.Fatalf("expected same timestamps ordered by name")
	}
}

func TestSelectProjectByNameOrIndex(t *testing.T) {
	cfg := config.GlobalConfig{
		Version: 1,
		Projects: map[string]config.ProjectEntry{
			"alpha": {LastUsed: "2025-01-02T00:00:00Z"},
			"bravo": {LastUsed: "2025-01-01T00:00:00Z"},
		},
	}

	name, err := SelectProject(cfg, "alpha")
	if err != nil || name != "alpha" {
		t.Fatalf("expected name selection, got %v/%s", err, name)
	}
	name, err = SelectProject(cfg, "1")
	if err != nil || name != "alpha" {
		t.Fatalf("expected index selection, got %v/%s", err, name)
	}
}

func TestSelectProjectErrors(t *testing.T) {
	cfg := config.GlobalConfig{
		Version:  1,
		Projects: map[string]config.ProjectEntry{},
	}
	if _, err := SelectProject(cfg, "1"); err == nil {
		t.Fatalf("expected error for empty project list")
	}

	cfg.Projects = map[string]config.ProjectEntry{
		"alpha": {LastUsed: "2025-01-02T00:00:00Z"},
	}
	if _, err := SelectProject(cfg, "0"); err == nil {
		t.Fatalf("expected invalid index error")
	}
	if _, err := SelectProject(cfg, "2"); err == nil {
		t.Fatalf("expected out of range error")
	}
	if _, err := SelectProject(cfg, "missing"); err == nil {
		t.Fatalf("expected missing project error")
	}
}
