// Where: cli/internal/state/app_state_test.go
// What: Tests for application-level project selection.
// Why: Validate ESB_PROJECT and last_used resolution logic.
package state

import (
	"os"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

func TestResolveAppStateUsesEnvVar(t *testing.T) {
	projects := map[string]config.ProjectEntry{
		"alpha": {Path: "/projects/alpha"},
		"beta":  {Path: "/projects/beta"},
	}

	state, err := ResolveAppState(AppStateOptions{
		ProjectEnv: "beta",
		Projects:   projects,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveProject != "beta" {
		t.Fatalf("unexpected project: %s", state.ActiveProject)
	}
}

func TestResolveAppStateUsesMostRecentProject(t *testing.T) {
	alpha := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beta := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	projects := map[string]config.ProjectEntry{
		"alpha": {Path: "/projects/alpha", LastUsed: alpha.Format(time.RFC3339)},
		"beta":  {Path: "/projects/beta", LastUsed: beta.Format(time.RFC3339)},
	}

	state, err := ResolveAppState(AppStateOptions{
		Projects: projects,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveProject != "beta" {
		t.Fatalf("unexpected project: %s", state.ActiveProject)
	}
}

func TestResolveAppStateErrorsWithoutRecentProject(t *testing.T) {
	projects := map[string]config.ProjectEntry{
		"alpha": {Path: "/projects/alpha"},
		"beta":  {Path: "/projects/beta"},
	}

	_, err := ResolveAppState(AppStateOptions{
		Projects: projects,
	})
	if err == nil {
		t.Fatalf("expected error when no recent project is available")
	}
}

func TestResolveAppStateForceUnsetsInvalidEnv(t *testing.T) {
	key := envutil.HostEnvKey(constants.HostSuffixProject)
	t.Setenv(key, "missing")
	projects := map[string]config.ProjectEntry{
		"alpha": {Path: "/projects/alpha", LastUsed: "2026-01-01T00:00:00Z"},
	}

	state, err := ResolveAppState(AppStateOptions{
		ProjectEnv: os.Getenv(key),
		Projects:   projects,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveProject != "alpha" {
		t.Fatalf("unexpected project: %s", state.ActiveProject)
	}
	if got := os.Getenv(key); got != "" {
		t.Fatalf("expected %s to be unset, got %q", key, got)
	}
}
