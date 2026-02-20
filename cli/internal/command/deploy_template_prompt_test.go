// Where: cli/internal/command/deploy_template_prompt_test.go
// What: Tests for SAM parameter prompt resolution and parsing helpers.
// Why: Keep deploy parameter prompting deterministic and regression-safe.
package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru-code/esb/cli/internal/infra/interaction"
)

type templateParamPrompter struct {
	inputs []string
	err    error
	titles []string
}

func (p *templateParamPrompter) Input(title string, _ []string) (string, error) {
	p.titles = append(p.titles, title)
	if p.err != nil {
		return "", p.err
	}
	if len(p.inputs) == 0 {
		return "", nil
	}
	value := p.inputs[0]
	p.inputs = p.inputs[1:]
	return value, nil
}

func (p *templateParamPrompter) Select(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *templateParamPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return "", nil
}

func TestPromptTemplateParametersNoParameters(t *testing.T) {
	templatePath := writePromptTemplateFile(t, "Resources: {}\n")

	got, err := promptTemplateParameters(templatePath, false, nil, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prompt template parameters: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty parameters, got %#v", got)
	}
}

func TestPromptTemplateParametersNonTTYUsesDefaultAndPrevious(t *testing.T) {
	templatePath := writePromptTemplateFile(t, `
Parameters:
  Alpha:
    Type: String
    Default: alpha-default
  Beta:
    Type: String
`)
	previous := map[string]string{"Beta": "beta-prev"}

	got, err := promptTemplateParameters(templatePath, false, nil, previous, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prompt template parameters: %v", err)
	}
	if got["Alpha"] != "alpha-default" {
		t.Fatalf("expected default value for Alpha, got %q", got["Alpha"])
	}
	if got["Beta"] != "beta-prev" {
		t.Fatalf("expected previous value for Beta, got %q", got["Beta"])
	}
}

func TestPromptTemplateParametersNonTTYRequiresValue(t *testing.T) {
	templatePath := writePromptTemplateFile(t, `
Parameters:
  RequiredParam:
    Type: Number
`)

	_, err := promptTemplateParameters(templatePath, false, nil, nil, &bytes.Buffer{})
	if !errors.Is(err, errParameterRequiresValue) {
		t.Fatalf("expected errParameterRequiresValue, got %v", err)
	}
	if !strings.Contains(err.Error(), "RequiredParam") {
		t.Fatalf("expected parameter name in error, got %v", err)
	}
}

func TestPromptTemplateParametersTTYSortedAndRetry(t *testing.T) {
	templatePath := writePromptTemplateFile(t, `
Parameters:
  Zeta:
    Type: String
  Alpha:
    Type: Number
`)
	prompter := &templateParamPrompter{
		inputs: []string{"", "42", ""},
	}
	var errOut bytes.Buffer

	got, err := promptTemplateParameters(templatePath, true, prompter, nil, &errOut)
	if err != nil {
		t.Fatalf("prompt template parameters: %v", err)
	}
	if got["Alpha"] != "42" {
		t.Fatalf("expected Alpha=42, got %q", got["Alpha"])
	}
	if got["Zeta"] != "" {
		t.Fatalf("expected Zeta empty string, got %q", got["Zeta"])
	}
	if len(prompter.titles) != 3 {
		t.Fatalf("expected 3 prompts (retry once), got %d", len(prompter.titles))
	}
	if !strings.Contains(prompter.titles[0], "Alpha") || !strings.Contains(prompter.titles[1], "Alpha") {
		t.Fatalf("expected Alpha prompt to retry first, got %#v", prompter.titles)
	}
	if !strings.Contains(prompter.titles[2], "Zeta") {
		t.Fatalf("expected Zeta prompt after Alpha, got %#v", prompter.titles)
	}
	if !strings.Contains(errOut.String(), `Parameter "Alpha" is required.`) {
		t.Fatalf("expected required warning, got %q", errOut.String())
	}
}

func TestPromptTemplateParametersReturnsPromptError(t *testing.T) {
	templatePath := writePromptTemplateFile(t, `
Parameters:
  Alpha:
    Type: String
`)
	prompter := &templateParamPrompter{err: errors.New("boom")}

	_, err := promptTemplateParameters(templatePath, true, prompter, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "prompt parameter Alpha: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractSAMParametersMapAnyAny(t *testing.T) {
	data := map[string]any{
		"Parameters": map[any]any{
			"MyParam": map[any]any{
				"Type":        "String",
				"Description": "description",
				"Default":     "default",
			},
			123: "ignored",
		},
	}

	params := extractSAMParameters(data)
	got, ok := params["MyParam"]
	if !ok {
		t.Fatalf("expected MyParam, got %#v", params)
	}
	if got.Type != "String" || got.Description != "description" || got.Default != "default" {
		t.Fatalf("unexpected param: %#v", got)
	}
}

func writePromptTemplateFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}
