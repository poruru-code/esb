// Where: cli/internal/command/deploy_inputs_resolve_test.go
// What: Integration-style unit tests for deploy input resolution orchestration.
// Why: Ensure runtime discovery failures are surfaced as warnings instead of being silently ignored.
package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

func TestResolveDeployInputsWarnsWhenRuntimeDiscoveryFails(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var errOut bytes.Buffer
	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:   "docker",
			NoSave: true,
		},
	}
	deps := Dependencies{
		ErrOut: &errOut,
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if strings.TrimSpace(inputs.Project) == "" {
		t.Fatalf("expected compose project to be resolved")
	}
	out := errOut.String()
	if !strings.Contains(out, "failed to discover running stacks") {
		t.Fatalf("expected stack discovery warning, got %q", out)
	}
	if !strings.Contains(out, "failed to infer runtime mode") {
		t.Fatalf("expected mode inference warning, got %q", out)
	}
}

func TestResolveDeployInputsAllowsTemplateOutsideRepo(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	templatePath := filepath.Join(t.TempDir(), "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			NoSave:  true,
		},
	}
	calledPaths := make([]string, 0, 2)
	deps := Dependencies{
		RepoResolver: func(path string) (string, error) {
			calledPaths = append(calledPaths, path)
			cleaned := filepath.Clean(path)
			if cleaned == "" || cleaned == "." {
				return repoRoot, nil
			}
			return "", errors.New("unexpected path")
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if inputs.ProjectDir != repoRoot {
		t.Fatalf("expected project dir %q, got %q", repoRoot, inputs.ProjectDir)
	}
	if len(inputs.Templates) != 1 || inputs.Templates[0].TemplatePath != templatePath {
		t.Fatalf("unexpected templates: %#v", inputs.Templates)
	}
	if len(calledPaths) == 0 {
		t.Fatal("expected repo resolver to be called")
	}
	for _, path := range calledPaths {
		if cleaned := filepath.Clean(path); cleaned != "" && cleaned != "." {
			t.Fatalf("repo resolver should be called with execution context only, got %q", path)
		}
	}
}

func TestResolveDeployInputsAllowsMultipleTemplatesOutsideRepo(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	templateA := filepath.Join(t.TempDir(), "template-a.yaml")
	templateB := filepath.Join(t.TempDir(), "template-b.yaml")
	if err := os.WriteFile(templateA, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template a: %v", err)
	}
	if err := os.WriteFile(templateB, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template b: %v", err)
	}

	cli := CLI{
		Template: []string{templateA, templateB},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			NoSave:  true,
		},
	}
	calledPaths := make([]string, 0, 4)
	deps := Dependencies{
		RepoResolver: func(path string) (string, error) {
			calledPaths = append(calledPaths, path)
			cleaned := filepath.Clean(path)
			if cleaned == "" || cleaned == "." {
				return repoRoot, nil
			}
			return "", errors.New("unexpected path")
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if inputs.ProjectDir != repoRoot {
		t.Fatalf("expected project dir %q, got %q", repoRoot, inputs.ProjectDir)
	}
	if len(inputs.Templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(inputs.Templates))
	}
	if inputs.Templates[0].TemplatePath != templateA || inputs.Templates[1].TemplatePath != templateB {
		t.Fatalf("unexpected template paths: %#v", inputs.Templates)
	}
	for _, path := range calledPaths {
		if cleaned := filepath.Clean(path); cleaned != "" && cleaned != "." {
			t.Fatalf("repo resolver should be called with execution context only, got %q", path)
		}
	}
}

func TestResolveDeployInputsSetsDefaultImageRuntimePerFunction(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	template := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ImageFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/repo:latest
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			NoSave:  true,
		},
	}
	deps := Dependencies{
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if len(inputs.Templates) != 1 {
		t.Fatalf("expected single template, got %d", len(inputs.Templates))
	}
	runtimes := inputs.Templates[0].ImageRuntimes
	if runtimes["lambda-image"] != "python3.12" {
		t.Fatalf("expected default runtime python3.12, got %#v", runtimes)
	}
}

func TestResolveDeployInputsUsesCLIImageRuntimeOverride(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	template := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ImageFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/repo:latest
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:         "docker",
			Project:      "esb-dev",
			ImageRuntime: []string{"lambda-image=java21"},
			NoSave:       true,
		},
	}
	deps := Dependencies{
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if len(inputs.Templates) != 1 {
		t.Fatalf("expected single template, got %d", len(inputs.Templates))
	}
	if got := inputs.Templates[0].ImageRuntimes["lambda-image"]; got != "java21" {
		t.Fatalf("expected runtime override java21, got %q", got)
	}
}

func TestResolveDeployInputsUsesCLIImageURIOverride(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	template := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ImageFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/repo:latest
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	override := "public.ecr.aws/example/override:v1"
	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:     "docker",
			Project:  "esb-dev",
			ImageURI: []string{"lambda-image=" + override},
			NoSave:   true,
		},
	}
	deps := Dependencies{
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if len(inputs.Templates) != 1 {
		t.Fatalf("expected single template, got %d", len(inputs.Templates))
	}
	if got := inputs.Templates[0].ImageSources["lambda-image"]; got != override {
		t.Fatalf("expected image uri override %q, got %q", override, got)
	}
}

func TestResolveDeployInputsUsesStoredImageRuntimeDefaults(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	template := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ImageFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/repo:latest
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cfgPath, err := config.ProjectConfigPath(repoRoot)
	if err != nil {
		t.Fatalf("project config path: %v", err)
	}
	cfg := config.DefaultGlobalConfig()
	cfg.BuildDefaults[templatePath] = config.BuildDefaults{
		Env:  "dev",
		Mode: "docker",
		ImageRuntimes: map[string]string{
			"lambda-image": "java21",
		},
	}
	if err := config.SaveGlobalConfig(cfgPath, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			NoSave:  true,
		},
	}
	deps := Dependencies{
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if len(inputs.Templates) != 1 {
		t.Fatalf("expected single template, got %d", len(inputs.Templates))
	}
	runtimes := inputs.Templates[0].ImageRuntimes
	if runtimes["lambda-image"] != "java21" {
		t.Fatalf("expected runtime java21 from stored defaults, got %#v", runtimes)
	}
}

type editFlowPrompter struct {
	selectValues      []string
	selectValueValues []string
	selectCalls       []imageRuntimeSelectCall
}

func (p *editFlowPrompter) Input(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *editFlowPrompter) Select(title string, options []string) (string, error) {
	p.selectCalls = append(p.selectCalls, imageRuntimeSelectCall{
		title:   title,
		options: append([]string{}, options...),
	})
	if len(p.selectValues) == 0 {
		return "", errors.New("no queued select value")
	}
	value := p.selectValues[0]
	p.selectValues = p.selectValues[1:]
	return value, nil
}

func (p *editFlowPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	if len(p.selectValueValues) == 0 {
		return "", errors.New("no queued select-value")
	}
	value := p.selectValueValues[0]
	p.selectValueValues = p.selectValueValues[1:]
	return value, nil
}

func TestResolveDeployInputsKeepsImageRuntimeAcrossEditLoop(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	template := `
Transform: AWS::Serverless-2016-10-31
Resources:
  ImageFn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-image
      PackageType: Image
      ImageUri: public.ecr.aws/example/repo:latest
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	prevIsTerminal := interaction.IsTerminal
	interaction.IsTerminal = func(_ *os.File) bool { return true }
	t.Cleanup(func() {
		interaction.IsTerminal = prevIsTerminal
	})

	prompter := &editFlowPrompter{
		selectValues:      []string{"java21", ""},
		selectValueValues: []string{"edit", "proceed"},
	}
	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			Output:  filepath.Join(repoRoot, ".esb", "out"),
			NoSave:  true,
		},
	}
	deps := Dependencies{
		Prompter: prompter,
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if len(inputs.Templates) != 1 {
		t.Fatalf("expected single template, got %d", len(inputs.Templates))
	}
	if got := inputs.Templates[0].ImageRuntimes["lambda-image"]; got != "java21" {
		t.Fatalf("expected retained image runtime java21, got %q", got)
	}
	if len(prompter.selectCalls) != 2 {
		t.Fatalf("expected 2 image runtime prompt calls, got %d", len(prompter.selectCalls))
	}
	if got := prompter.selectCalls[1].options[0]; got != "java21" {
		t.Fatalf("expected previous runtime to be default on second pass, got %q", got)
	}
}
