package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

type imageRuntimePrompter struct {
	selectValues []string
	selectCalls  []imageRuntimeSelectCall
}

type imageRuntimeSelectCall struct {
	title   string
	options []string
}

func (p *imageRuntimePrompter) Input(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *imageRuntimePrompter) Select(title string, options []string) (string, error) {
	p.selectCalls = append(p.selectCalls, imageRuntimeSelectCall{
		title:   title,
		options: append([]string{}, options...),
	})
	if len(p.selectValues) == 0 {
		return "", fmt.Errorf("no queued selection")
	}
	value := p.selectValues[0]
	p.selectValues = p.selectValues[1:]
	return value, nil
}

func (p *imageRuntimePrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return "", nil
}

func TestPromptTemplateImageRuntimesNonTTYUsesPreviousAndDefault(t *testing.T) {
	templatePath := writeImageRuntimeTemplate(t)

	var errOut bytes.Buffer
	got, err := promptTemplateImageRuntimes(
		templatePath,
		nil,
		false,
		nil,
		map[string]string{
			"a-image": "java21",
			"b-image": "invalid",
		},
		nil,
		&errOut,
	)
	if err != nil {
		t.Fatalf("prompt image runtimes: %v", err)
	}

	want := map[string]string{
		"a-image": "java21",
		"b-image": "python3.12",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runtimes: got=%v want=%v", got, want)
	}
	if !strings.Contains(errOut.String(), "Ignoring previous runtime") {
		t.Fatalf("expected warning for invalid previous runtime, got %q", errOut.String())
	}
}

func TestPromptTemplateImageRuntimesTTYPromptsEachImageFunction(t *testing.T) {
	templatePath := writeImageRuntimeTemplate(t)

	prompter := &imageRuntimePrompter{
		selectValues: []string{"java21", "python"},
	}
	got, err := promptTemplateImageRuntimes(
		templatePath,
		nil,
		true,
		prompter,
		map[string]string{
			"a-image": "python3.12",
			"b-image": "java21",
		},
		nil,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("prompt image runtimes: %v", err)
	}

	want := map[string]string{
		"a-image": "java21",
		"b-image": "python3.12",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runtimes: got=%v want=%v", got, want)
	}
	if len(prompter.selectCalls) != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", len(prompter.selectCalls))
	}
	if !strings.Contains(prompter.selectCalls[0].title, "a-image") {
		t.Fatalf("expected first prompt for a-image, got %q", prompter.selectCalls[0].title)
	}
	if !reflect.DeepEqual(prompter.selectCalls[1].options, []string{"java21", "python"}) {
		t.Fatalf("unexpected options for b-image: %v", prompter.selectCalls[1].options)
	}
}

func TestPromptTemplateImageRuntimesTTYUsesExplicitWithoutPrompt(t *testing.T) {
	templatePath := writeImageRuntimeTemplate(t)
	prompter := &imageRuntimePrompter{}

	got, err := promptTemplateImageRuntimes(
		templatePath,
		nil,
		true,
		prompter,
		nil,
		map[string]string{
			"a-image": "java21",
			"b-image": "python",
		},
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("prompt image runtimes: %v", err)
	}
	want := map[string]string{
		"a-image": "java21",
		"b-image": "python3.12",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runtimes: got=%v want=%v", got, want)
	}
	if len(prompter.selectCalls) != 0 {
		t.Fatalf("expected no prompt call, got %d", len(prompter.selectCalls))
	}
}

func TestPromptTemplateImageRuntimesExplicitInvalidReturnsError(t *testing.T) {
	templatePath := writeImageRuntimeTemplate(t)
	prompter := &imageRuntimePrompter{}

	_, err := promptTemplateImageRuntimes(
		templatePath,
		nil,
		true,
		prompter,
		nil,
		map[string]string{
			"a-image": "nodejs20.x",
		},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected error for invalid explicit runtime")
	}
	if len(prompter.selectCalls) != 0 {
		t.Fatalf("expected no prompt call on explicit runtime path, got %d", len(prompter.selectCalls))
	}
}

func TestPromptTemplateImageRuntimesReturnsNilWhenNoImageFunctions(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "template.yaml")
	content := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ZipFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: zip-fn
      Runtime: python3.12
      Handler: app.handler
      CodeUri: src/
`
	if err := os.WriteFile(templatePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got, err := promptTemplateImageRuntimes(templatePath, nil, false, nil, nil, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prompt image runtimes: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil runtimes, got %v", got)
	}
}

func TestNormalizeImageRuntimeSelection(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "", want: "python3.12", ok: true},
		{input: "python", want: "python3.12", ok: true},
		{input: "python3.12", want: "python3.12", ok: true},
		{input: "java21", want: "java21", ok: true},
		{input: "nodejs20.x", ok: false},
	}

	for _, tc := range cases {
		got, err := normalizeImageRuntimeSelection(tc.input)
		if tc.ok && err != nil {
			t.Fatalf("normalize %q: %v", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("normalize %q: expected error", tc.input)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("normalize %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}

func writeImageRuntimeTemplate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.yaml")
	content := `
Transform: AWS::Serverless-2016-10-31
Resources:
  BImage:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: b-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/b:latest
  AImage:
    Type: AWS::Lambda::Function
    Properties:
      FunctionName: a-image
      PackageType: Image
      Code:
        ImageUri: public.ecr.aws/example/a:latest
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}
