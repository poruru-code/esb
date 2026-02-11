// Where: cli/internal/command/deploy_summary_test.go
// What: Tests for deploy summary rendering and confirmation behavior.
// Why: Ensure confirmation prompts stay deterministic and safe to edit.
package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

type summaryPrompter struct {
	choice  string
	err     error
	summary string
	options []interaction.SelectOption
}

func (p *summaryPrompter) Input(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *summaryPrompter) Select(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *summaryPrompter) SelectValue(summary string, options []interaction.SelectOption) (string, error) {
	p.summary = summary
	p.options = options
	if p.err != nil {
		return "", p.err
	}
	return p.choice, nil
}

func TestConfirmDeployInputsNonTTYSkipsPrompt(t *testing.T) {
	ok, err := confirmDeployInputs(deployInputs{}, false, nil)
	if err != nil {
		t.Fatalf("confirm deploy inputs: %v", err)
	}
	if !ok {
		t.Fatal("expected true when prompt is skipped")
	}
}

func TestConfirmDeployInputsProceedAndSummaryContents(t *testing.T) {
	templatePath := writeSummaryTemplate(t)
	prompter := &summaryPrompter{choice: "proceed"}
	inputs := deployInputs{
		TargetStack:   "esb-dev",
		Project:       "esb-dev",
		ProjectSource: "stack",
		Env:           "dev",
		EnvSource:     "stack",
		Mode:          "docker",
		Templates: []deployTemplateInput{
			{
				TemplatePath: templatePath,
				OutputDir:    ".esb/out/template",
				Parameters: map[string]string{
					"B": "2",
					"A": "1",
				},
			},
		},
	}

	ok, err := confirmDeployInputs(inputs, true, prompter)
	if err != nil {
		t.Fatalf("confirm deploy inputs: %v", err)
	}
	if !ok {
		t.Fatal("expected proceed to return true")
	}
	if len(prompter.options) != 2 {
		t.Fatalf("expected 2 confirmation options, got %d", len(prompter.options))
	}
	if !strings.Contains(prompter.summary, "Target Stack: esb-dev") {
		t.Fatalf("missing target stack in summary: %q", prompter.summary)
	}
	if !strings.Contains(prompter.summary, "Project: esb-dev (stack)") {
		t.Fatalf("missing project source in summary: %q", prompter.summary)
	}
	if !strings.Contains(prompter.summary, "Env: dev (stack)") {
		t.Fatalf("missing env source in summary: %q", prompter.summary)
	}
	idxA := strings.Index(prompter.summary, "  A = 1")
	idxB := strings.Index(prompter.summary, "  B = 2")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("expected sorted params in summary, got %q", prompter.summary)
	}
}

func TestConfirmDeployInputsEditReturnsFalse(t *testing.T) {
	ok, err := confirmDeployInputs(deployInputs{}, true, &summaryPrompter{choice: "edit"})
	if err != nil {
		t.Fatalf("confirm deploy inputs: %v", err)
	}
	if ok {
		t.Fatal("expected edit choice to return false")
	}
}

func TestConfirmDeployInputsReturnsPromptError(t *testing.T) {
	_, err := confirmDeployInputs(deployInputs{}, true, &summaryPrompter{err: errors.New("boom")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "prompt confirmation: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendTemplateSummaryLinesDeterministicParams(t *testing.T) {
	templatePath := writeSummaryTemplate(t)
	lines := appendTemplateSummaryLines(nil, deployTemplateInput{
		TemplatePath: templatePath,
		OutputDir:    ".esb/out/template",
		Parameters: map[string]string{
			"Z": "last",
			"A": "first",
		},
	}, "dev", "esb-dev")
	joined := strings.Join(lines, "\n")
	idxA := strings.Index(joined, "  A = first")
	idxZ := strings.Index(joined, "  Z = last")
	if idxA < 0 || idxZ < 0 || idxA > idxZ {
		t.Fatalf("expected sorted parameter lines, got %q", joined)
	}
}

func writeSummaryTemplate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.yaml")
	if err := os.WriteFile(path, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}
