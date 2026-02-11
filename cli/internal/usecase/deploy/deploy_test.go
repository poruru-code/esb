package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	infradeploy "github.com/poruru/edge-serverless-box/cli/internal/infra/deploy"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func TestDeployWorkflowRunSuccess(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	// Use the actual repo root for testing
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Go up to the repo root (we're in cli/internal/usecase/deploy)
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	req := Request{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		OutputDir:    ".out",
		Parameters:   map[string]string{"ParamA": "value"},
		Tag:          "v1.2.3",
		NoCache:      true,
	}

	workflow := NewDeployWorkflow(builder.Build, envApplier.Apply, ui, runner)
	// Use a mock registry waiter to avoid waiting for real registry
	workflow.RegistryWaiter = noopRegistryWaiter
	// Note: This test uses the actual repo root. It will fail if the repo
	// structure doesn't match expectations (e.g., missing compose files).
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.applied) != 1 {
		t.Fatalf("expected env applier to be called once, got %d", len(envApplier.applied))
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called once, got %d", len(builder.requests))
	}

	got := builder.requests[0]
	if got.ProjectDir != req.Context.ProjectDir {
		t.Fatalf("project dir mismatch: %s", got.ProjectDir)
	}
	if got.ProjectName != req.Context.ComposeProject {
		t.Fatalf("project name mismatch: %s", got.ProjectName)
	}
	if got.TemplatePath != req.TemplatePath {
		t.Fatalf("template path mismatch: %s", got.TemplatePath)
	}
	if got.Env != req.Env {
		t.Fatalf("env mismatch: %s", got.Env)
	}
	if got.Mode != req.Mode {
		t.Fatalf("mode mismatch: %s", got.Mode)
	}
	if got.OutputDir != req.OutputDir {
		t.Fatalf("output dir mismatch: %s", got.OutputDir)
	}
	if got.Tag != req.Tag {
		t.Fatalf("tag mismatch: %s", got.Tag)
	}
	if got.Parameters["ParamA"] != "value" {
		t.Fatalf("parameters mismatch")
	}
	if !got.NoCache {
		t.Fatalf("expected no-cache to be true")
	}

	if len(ui.success) != 1 || !strings.Contains(ui.success[0], "Deploy complete") {
		t.Fatalf("expected deploy success message")
	}
}

func TestDeployWorkflowRequiresPrewarmForImageFunctions(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	templatePath := filepath.Join(repoRoot, "template.yaml")
	configDir, err := staging.ConfigDir(templatePath, "esb-dev", "dev")
	if err != nil {
		t.Fatalf("failed to resolve staging config dir: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	manifestPath := filepath.Join(configDir, "image-import.json")
	manifest := `{"version":"1","images":[{"function_name":"lambda-image","image_source":"public.ecr.aws/example/repo:latest","image_ref":"registry:5010/public.ecr.aws/example/repo:latest"}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write image-import.json: %v", err)
	}

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	req := Request{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: templatePath,
		OutputDir:    ".out",
		ImagePrewarm: "off",
	}

	workflow := NewDeployWorkflow(builder.Build, envApplier.Apply, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	err = workflow.Run(req)
	if err == nil || !strings.Contains(err.Error(), "image prewarm is required") {
		t.Fatalf("expected prewarm required error, got %v", err)
	}
}

func TestDeployWorkflowRunWithExternalTemplate(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	externalDir := t.TempDir()
	externalTemplate := filepath.Join(externalDir, "template.yaml")
	if err := os.WriteFile(externalTemplate, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write external template: %v", err)
	}

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	req := Request{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: externalTemplate,
		OutputDir:    ".out",
		Tag:          "v1.2.3",
		BuildOnly:    true,
	}

	workflow := NewDeployWorkflow(builder.Build, envApplier.Apply, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called once, got %d", len(builder.requests))
	}
	if builder.requests[0].TemplatePath != externalTemplate {
		t.Fatalf("template path mismatch: %s", builder.requests[0].TemplatePath)
	}
}

func TestDeployWorkflowRunMissingBuilder(t *testing.T) {
	workflow := NewDeployWorkflow(nil, nil, nil, nil)
	err := workflow.Run(Request{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "builder is not configured") {
		t.Fatalf("expected builder missing error, got %v", err)
	}
}

func TestRunProvisionerUsesComposeOverride(t *testing.T) {
	runner := &fakeComposeRunner{}
	ui := &testUI{}
	workflow := Workflow{
		ComposeRunner:      runner,
		UserInterface:      ui,
		ComposeProvisioner: infradeploy.NewComposeProvisioner(runner, ui),
	}
	t.Setenv("ENV_PREFIX", "ESB")

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  provisioner: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	if err := workflow.runProvisioner(
		"esb-test",
		"docker",
		false,
		false,
		repoRoot,
		[]string{composePath},
	); err != nil {
		t.Fatalf("runProvisioner: %v", err)
	}

	foundConfig := false
	foundRun := false
	for _, cmd := range runner.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "config --services") && strings.Contains(joined, composePath) {
			foundConfig = true
		}
		if strings.Contains(joined, "run --rm provisioner") && strings.Contains(joined, composePath) {
			foundRun = true
		}
	}
	if !foundConfig {
		t.Fatalf("expected compose config to use override file")
	}
	if !foundRun {
		t.Fatalf("expected compose run to use override file")
	}
}

func TestRunProvisionerFailsOnOverrideMissingServices(t *testing.T) {
	runner := &fakeComposeRunner{output: []byte("provisioner\n")}
	ui := &testUI{}
	workflow := Workflow{
		ComposeRunner:      runner,
		UserInterface:      ui,
		ComposeProvisioner: infradeploy.NewComposeProvisioner(runner, ui),
	}
	t.Setenv("ENV_PREFIX", "ESB")

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  provisioner: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	err = workflow.runProvisioner(
		"esb-test",
		"docker",
		false,
		false,
		repoRoot,
		[]string{composePath},
	)
	if err == nil || !strings.Contains(err.Error(), "compose override missing services") {
		t.Fatalf("expected missing services error, got %v", err)
	}
}

func TestRunProvisionerWithNoDepsAddsFlag(t *testing.T) {
	runner := &fakeComposeRunner{}
	ui := &testUI{}
	workflow := Workflow{
		ComposeRunner:      runner,
		UserInterface:      ui,
		ComposeProvisioner: infradeploy.NewComposeProvisioner(runner, ui),
	}
	t.Setenv("ENV_PREFIX", "ESB")

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  provisioner: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	if err := workflow.runProvisioner(
		"esb-test",
		"docker",
		true,
		false,
		repoRoot,
		[]string{composePath},
	); err != nil {
		t.Fatalf("runProvisioner: %v", err)
	}

	foundRun := false
	for _, cmd := range runner.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "run --rm --no-deps provisioner") {
			foundRun = true
			break
		}
	}
	if !foundRun {
		t.Fatalf("expected compose run to include --no-deps")
	}
}
