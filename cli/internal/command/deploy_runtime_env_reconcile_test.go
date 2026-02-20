// Where: cli/internal/command/deploy_runtime_env_reconcile_test.go
// What: Unit tests for deploy env reconciliation with runtime inference.
// Why: Freeze mismatch resolution behavior before further refactors.
package command

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/poruru-code/esb/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru-code/esb/cli/internal/infra/runtime"
)

type fixedEnvResolver struct {
	inferred runtimeinfra.EnvInference
	err      error
}

func (r fixedEnvResolver) InferEnvFromProject(_, _ string) (runtimeinfra.EnvInference, error) {
	return r.inferred, r.err
}

type selectOnlyPrompter struct {
	selected string
}

func (p selectOnlyPrompter) Input(_ string, _ []string) (string, error) {
	return "", nil
}

func (p selectOnlyPrompter) Select(_ string, _ []string) (string, error) {
	return "", nil
}

func (p selectOnlyPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return p.selected, nil
}

func TestReconcileEnvWithRuntimeKeepsExplicitWhenForceEnabled(t *testing.T) {
	choice := envChoice{Value: "prod", Source: "flag", Explicit: true}
	got, err := reconcileEnvWithRuntime(
		choice,
		"esb-dev",
		"template.yaml",
		false,
		nil,
		fixedEnvResolver{inferred: runtimeinfra.EnvInference{Env: "dev", Source: "container label"}},
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("reconcile env: %v", err)
	}
	if got != choice {
		t.Fatalf("expected explicit choice to be kept, got %#v", got)
	}
}

func TestReconcileEnvWithRuntimeAlignsImplicitChoice(t *testing.T) {
	got, err := reconcileEnvWithRuntime(
		envChoice{Value: "prod", Source: "default", Explicit: false},
		"esb-dev",
		"template.yaml",
		false,
		nil,
		fixedEnvResolver{inferred: runtimeinfra.EnvInference{Env: "dev", Source: "gateway env"}},
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("reconcile env: %v", err)
	}
	if got.Value != "dev" {
		t.Fatalf("expected aligned env 'dev', got %q", got.Value)
	}
	if got.Source != "gateway env" {
		t.Fatalf("expected source 'gateway env', got %q", got.Source)
	}
	if got.Explicit {
		t.Fatalf("expected implicit choice, got explicit=true")
	}
}

func TestReconcileEnvWithRuntimeErrorsForExplicitMismatchWithoutTTY(t *testing.T) {
	_, err := reconcileEnvWithRuntime(
		envChoice{Value: "prod", Source: "flag", Explicit: true},
		"esb-dev",
		"template.yaml",
		false,
		nil,
		fixedEnvResolver{inferred: runtimeinfra.EnvInference{Env: "dev", Source: "staging"}},
		false,
		nil,
	)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !errors.Is(err, errEnvMismatch) {
		t.Fatalf("expected errEnvMismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "running env uses") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestReconcileEnvWithRuntimeUsesPromptSelection(t *testing.T) {
	got, err := reconcileEnvWithRuntime(
		envChoice{Value: "prod", Source: "flag", Explicit: true},
		"esb-dev",
		"template.yaml",
		true,
		selectOnlyPrompter{selected: "dev"},
		fixedEnvResolver{inferred: runtimeinfra.EnvInference{Env: "dev", Source: "container label"}},
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("reconcile env: %v", err)
	}
	if got.Value != "dev" {
		t.Fatalf("expected selected inferred env, got %q", got.Value)
	}
	if got.Source != "container label" {
		t.Fatalf("expected inferred source, got %q", got.Source)
	}
	if !got.Explicit {
		t.Fatalf("expected explicit=true after prompt selection")
	}
}

func TestApplyEnvSelectionKeepsCurrentAndMarksPromptSource(t *testing.T) {
	current := envChoice{Value: "prod", Source: "default", Explicit: false}
	inferred := runtimeinfra.EnvInference{Env: "dev", Source: "container label"}

	got := applyEnvSelection(current, inferred, "prod")
	if got.Value != "prod" {
		t.Fatalf("expected current env to be kept, got %q", got.Value)
	}
	if got.Source != "prompt" {
		t.Fatalf("expected source to switch to prompt, got %q", got.Source)
	}
	if !got.Explicit {
		t.Fatalf("expected explicit=true after manual keep")
	}
}

func TestApplyEnvSelectionUsesInferredWhenSelected(t *testing.T) {
	current := envChoice{Value: "prod", Source: "flag", Explicit: true}
	inferred := runtimeinfra.EnvInference{Env: "dev", Source: "container label"}

	got := applyEnvSelection(current, inferred, "dev")
	if got.Value != "dev" {
		t.Fatalf("expected inferred env, got %q", got.Value)
	}
	if got.Source != "container label" {
		t.Fatalf("expected inferred source, got %q", got.Source)
	}
	if !got.Explicit {
		t.Fatalf("expected explicit=true")
	}
}

func TestReconcileEnvWithRuntimeSkipsWhenProjectEmpty(t *testing.T) {
	choice := envChoice{Value: "dev", Source: "flag", Explicit: true}
	got, err := reconcileEnvWithRuntime(
		choice,
		"",
		"template.yaml",
		false,
		nil,
		fixedEnvResolver{inferred: runtimeinfra.EnvInference{Env: "prod", Source: "runtime"}},
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("reconcile env: %v", err)
	}
	if got != choice {
		t.Fatalf("expected choice unchanged, got %#v", got)
	}
}

func TestReconcileEnvWithRuntimeResolverErrorWritesWarning(t *testing.T) {
	choice := envChoice{Value: "dev", Source: "flag", Explicit: true}
	var errOut bytes.Buffer
	got, err := reconcileEnvWithRuntime(
		choice,
		"esb-dev",
		"template.yaml",
		false,
		nil,
		fixedEnvResolver{err: errors.New("boom")},
		false,
		&errOut,
	)
	if err != nil {
		t.Fatalf("reconcile env: %v", err)
	}
	if got != choice {
		t.Fatalf("expected choice unchanged, got %#v", got)
	}
	if !strings.Contains(errOut.String(), "failed to infer running env") {
		t.Fatalf("expected warning output, got %q", errOut.String())
	}
}
