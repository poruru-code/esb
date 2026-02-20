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

	"github.com/poruru-code/esb/cli/internal/infra/interaction"
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
	repoRoot := t.TempDir()
	setWorkingDirForIsolation(t, repoRoot)
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
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
				ImageRuntimes: map[string]string{
					"b-image": "python3.12",
					"a-image": "java21",
				},
				ImageSources: map[string]string{
					"b-image": "public.ecr.aws/example/b:latest",
					"a-image": "public.ecr.aws/example/a:latest",
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
	idxRuntimeA := strings.Index(prompter.summary, "  a-image = java21")
	idxRuntimeB := strings.Index(prompter.summary, "  b-image = python")
	if idxRuntimeA < 0 || idxRuntimeB < 0 || idxRuntimeA > idxRuntimeB {
		t.Fatalf("expected sorted image runtimes in summary, got %q", prompter.summary)
	}
	idxSourceA := strings.Index(prompter.summary, "  a-image = public.ecr.aws/example/a:latest")
	idxSourceB := strings.Index(prompter.summary, "  b-image = public.ecr.aws/example/b:latest")
	if idxSourceA < 0 || idxSourceB < 0 || idxSourceA > idxSourceB {
		t.Fatalf("expected sorted image sources in summary, got %q", prompter.summary)
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
	repoRoot := t.TempDir()
	setWorkingDirForIsolation(t, repoRoot)
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := writeSummaryTemplate(t)
	lines := appendTemplateSummaryLines(nil, deployTemplateInput{
		TemplatePath: templatePath,
		OutputDir:    ".esb/out/template",
		Parameters: map[string]string{
			"Z": "last",
			"A": "first",
		},
		ImageRuntimes: map[string]string{
			"z-image": "python3.12",
			"a-image": "java21",
		},
		ImageSources: map[string]string{
			"z-image": "public.ecr.aws/example/z:latest",
			"a-image": "public.ecr.aws/example/a:latest",
		},
	}, "dev", "esb-dev")
	joined := strings.Join(lines, "\n")
	idxA := strings.Index(joined, "  A = first")
	idxZ := strings.Index(joined, "  Z = last")
	if idxA < 0 || idxZ < 0 || idxA > idxZ {
		t.Fatalf("expected sorted parameter lines, got %q", joined)
	}
	idxRuntimeA := strings.Index(joined, "  a-image = java21")
	idxRuntimeZ := strings.Index(joined, "  z-image = python")
	if idxRuntimeA < 0 || idxRuntimeZ < 0 || idxRuntimeA > idxRuntimeZ {
		t.Fatalf("expected sorted runtime lines, got %q", joined)
	}
	idxSourceA := strings.Index(joined, "  a-image = public.ecr.aws/example/a:latest")
	idxSourceZ := strings.Index(joined, "  z-image = public.ecr.aws/example/z:latest")
	if idxSourceA < 0 || idxSourceZ < 0 || idxSourceA > idxSourceZ {
		t.Fatalf("expected sorted source lines, got %q", joined)
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

func setWorkingDirForIsolation(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}
