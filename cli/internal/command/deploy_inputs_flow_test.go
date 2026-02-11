// Where: cli/internal/command/deploy_inputs_flow_test.go
// What: Flow-oriented unit tests for deploy input resolution helpers.
// Why: Keep stack-first env resolution behavior stable for interactive deploy UX.
package command

import (
	"testing"

	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
)

func TestResolveDeployEnvFromStackPrefersFlag(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "ignored"}
	got, err := resolveDeployEnvFromStack(
		"prod",
		deployTargetStack{Name: "esb-dev", Env: "dev"},
		"esb3",
		true,
		prompter,
		nil,
		"default",
	)
	if err != nil {
		t.Fatalf("resolve env from stack: %v", err)
	}
	if got.Value != "prod" {
		t.Fatalf("expected env prod, got %q", got.Value)
	}
	if got.Source != "flag" {
		t.Fatalf("expected source flag, got %q", got.Source)
	}
	if prompter.inputCalls != 0 {
		t.Fatalf("expected no prompt call, got %d", prompter.inputCalls)
	}
}

func TestResolveDeployEnvFromStackUsesStackEnvWithoutPrompt(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "ignored"}
	got, err := resolveDeployEnvFromStack(
		"",
		deployTargetStack{Name: "esb-dev", Env: "dev"},
		"",
		true,
		prompter,
		nil,
		"default",
	)
	if err != nil {
		t.Fatalf("resolve env from stack: %v", err)
	}
	if got.Value != "dev" {
		t.Fatalf("expected env dev, got %q", got.Value)
	}
	if got.Source != "stack" {
		t.Fatalf("expected source stack, got %q", got.Source)
	}
	if prompter.inputCalls != 0 {
		t.Fatalf("expected no prompt call, got %d", prompter.inputCalls)
	}
}

func TestResolveDeployEnvFromStackFallsBackToPrompt(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "staging"}
	got, err := resolveDeployEnvFromStack(
		"",
		deployTargetStack{},
		"",
		true,
		prompter,
		nil,
		"default",
	)
	if err != nil {
		t.Fatalf("resolve env from stack: %v", err)
	}
	if got.Value != "staging" {
		t.Fatalf("expected env staging, got %q", got.Value)
	}
	if got.Source != "prompt" {
		t.Fatalf("expected source prompt, got %q", got.Source)
	}
	if prompter.inputCalls != 1 {
		t.Fatalf("expected one prompt call, got %d", prompter.inputCalls)
	}
}

func TestResolveDeployEnvFromStackUsesRuntimeResolver(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "ignored"}
	got, err := resolveDeployEnvFromStack(
		"",
		deployTargetStack{},
		"esb-dev",
		true,
		prompter,
		fixedEnvResolver{
			inferred: runtimeinfra.EnvInference{Env: "qa", Source: "container label"},
		},
		"default",
	)
	if err != nil {
		t.Fatalf("resolve env from stack: %v", err)
	}
	if got.Value != "qa" {
		t.Fatalf("expected env qa, got %q", got.Value)
	}
	if got.Source != "container label" {
		t.Fatalf("expected source container label, got %q", got.Source)
	}
	if prompter.inputCalls != 0 {
		t.Fatalf("expected no prompt call, got %d", prompter.inputCalls)
	}
}

func TestResolveDeployOutputInteractiveUsesPreviousWithoutPrompt(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "should-not-be-used"}
	got, err := resolveDeployOutput(
		"",
		"e2e/fixtures/template.core.yaml",
		"dev",
		true,
		prompter,
		".esb/custom",
	)
	if err != nil {
		t.Fatalf("resolve deploy output: %v", err)
	}
	if got != ".esb/custom" {
		t.Fatalf("expected previous output, got %q", got)
	}
	if prompter.inputCalls != 0 {
		t.Fatalf("expected no prompt call, got %d", prompter.inputCalls)
	}
}

func TestResolveDeployOutputInteractiveUsesDefaultWithoutPrompt(t *testing.T) {
	prompter := &recordingPrompter{inputValue: "should-not-be-used"}
	got, err := resolveDeployOutput(
		"",
		"e2e/fixtures/template.core.yaml",
		"dev",
		true,
		prompter,
		"",
	)
	if err != nil {
		t.Fatalf("resolve deploy output: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty output (default path), got %q", got)
	}
	if prompter.inputCalls != 0 {
		t.Fatalf("expected no prompt call, got %d", prompter.inputCalls)
	}
}
