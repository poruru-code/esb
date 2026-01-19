// Where: cli/internal/state/project_state_test.go
// What: Tests for environment selection within a project.
// Why: Ensure ESB_ENV, last_env, and single-env defaults behave correctly.
package state

import (
	"os"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

func TestResolveProjectStateUsesEnvFlag(t *testing.T) {
	cfg := config.GeneratorConfig{
		Environments: config.Environments{
			{Name: "default", Mode: "docker"},
			{Name: "staging", Mode: "containerd"},
		},
	}

	state, err := ResolveProjectState(ProjectStateOptions{
		EnvFlag: "staging",
		Config:  cfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveEnv != "staging" {
		t.Fatalf("unexpected env: %s", state.ActiveEnv)
	}
}

func TestResolveProjectStateUsesLastEnv(t *testing.T) {
	cfg := config.GeneratorConfig{
		App: config.AppConfig{LastEnv: "staging"},
		Environments: config.Environments{
			{Name: "default", Mode: "docker"},
			{Name: "staging", Mode: "containerd"},
		},
	}

	state, err := ResolveProjectState(ProjectStateOptions{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveEnv != "staging" {
		t.Fatalf("unexpected env: %s", state.ActiveEnv)
	}
}

func TestResolveProjectStateForceUnsetsInvalidEnv(t *testing.T) {
	key := envutil.HostEnvKey(constants.HostSuffixEnv)
	t.Setenv(key, "missing")
	opts := ProjectStateOptions{
		EnvVar: os.Getenv(key),
		Config: config.GeneratorConfig{
			Environments: config.Environments{{Name: "prod"}},
		},
		Force: true,
	}

	state, err := ResolveProjectState(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveEnv != "prod" {
		t.Fatalf("unexpected env: %s", state.ActiveEnv)
	}
	if got := os.Getenv(key); got != "" {
		t.Fatalf("expected %s to be unset, got %q", key, got)
	}
}

func TestResolveProjectStateUsesSingleEnv(t *testing.T) {
	cfg := config.GeneratorConfig{
		Environments: config.Environments{{Name: "default", Mode: "docker"}},
	}

	state, err := ResolveProjectState(ProjectStateOptions{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ActiveEnv != "default" {
		t.Fatalf("unexpected env: %s", state.ActiveEnv)
	}
}

func TestResolveProjectStateErrorsWithoutActiveEnv(t *testing.T) {
	cfg := config.GeneratorConfig{
		Environments: config.Environments{
			{Name: "default", Mode: "docker"},
			{Name: "staging", Mode: "containerd"},
		},
	}

	_, err := ResolveProjectState(ProjectStateOptions{
		Config: cfg,
	})
	if err == nil {
		t.Fatalf("expected error when no active env is available")
	}
}
