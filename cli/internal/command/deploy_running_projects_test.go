// Where: cli/internal/command/deploy_running_projects_test.go
// What: Unit tests for deploy running project discovery helpers.
// Why: Ensure project filtering and env inference from labels behave deterministically.
package command

import (
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

func TestInferEnvFromContainerLabels(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "dev" {
			t.Fatalf("expected env 'dev', got %q", got.Env)
		}
		if got.Source != "container label" {
			t.Fatalf("expected source 'container label', got %q", got.Source)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
			{Labels: map[string]string{compose.ESBEnvLabel: "prod"}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})

	t.Run("none", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})
}

func TestRunningProjectsFilteredByEnv(t *testing.T) {
	containers := []container.Summary{
		{Labels: map[string]string{compose.ComposeProjectLabel: "proj-a", compose.ComposeServiceLabel: "gateway"}},
		{Labels: map[string]string{compose.ComposeProjectLabel: "proj-b", compose.ComposeServiceLabel: "other"}},
		{Labels: map[string]string{compose.ComposeProjectLabel: "proj-c", compose.ComposeServiceLabel: "runtime-node"}},
		{Labels: map[string]string{compose.ComposeProjectLabel: "proj-d", compose.ComposeServiceLabel: "database"}},
		{Labels: map[string]string{compose.ComposeProjectLabel: "proj-e", compose.ComposeServiceLabel: "custom", compose.ESBManagedLabel: "true"}},
	}

	got := filterRunningProjectsByEnv(containers, runningProjectServices, func(project string) envInference {
		switch project {
		case "proj-a":
			return envInference{Env: "dev"}
		case "proj-d":
			return envInference{Env: "prod"}
		case "proj-e":
			return envInference{Env: "staging"}
		default:
			return envInference{}
		}
	})

	want := []string{"proj-a", "proj-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected projects: %v", got)
	}
}
