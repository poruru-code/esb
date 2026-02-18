package deploy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
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

	repoRoot := newTestRepoRoot(t)

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	artifactPath := writeTestArtifactManifest(t, false)
	req := Request{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		ArtifactPath: artifactPath,
		OutputDir:    ".out",
		Parameters:   map[string]string{"ParamA": "value"},
		ImageSources: map[string]string{
			"lambda-image": "public.ecr.aws/example/repo:latest",
		},
		ImageRuntimes: map[string]string{
			"lambda-image": "java21",
		},
		Tag:     "v1.2.3",
		NoCache: true,
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
	if got.ImageRuntimes["lambda-image"] != "java21" {
		t.Fatalf("image runtime mismatch: %#v", got.ImageRuntimes)
	}
	if got.ImageSources["lambda-image"] != "public.ecr.aws/example/repo:latest" {
		t.Fatalf("image source mismatch: %#v", got.ImageSources)
	}
	if !got.NoCache {
		t.Fatalf("expected no-cache to be true")
	}
	if !got.BuildImages {
		t.Fatalf("expected build images to default true")
	}

	if len(ui.success) != 1 || !strings.Contains(ui.success[0], "Deploy complete") {
		t.Fatalf("expected deploy success message")
	}
}

func TestDeployWorkflowRunRespectsRenderOnlyBuildFlag(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	repoRoot := newTestRepoRoot(t)
	buildImages := false
	req := Request{
		Context: state.Context{
			ProjectDir:     repoRoot,
			ComposeProject: "esb-dev",
		},
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		OutputDir:    ".out",
		BuildOnly:    true,
		BuildImages:  &buildImages,
	}

	workflow := NewDeployWorkflow(builder.Build, envApplier.Apply, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called once, got %d", len(builder.requests))
	}
	if builder.requests[0].BuildImages {
		t.Fatalf("expected BuildImages=false to be forwarded")
	}
}

func TestDeployWorkflowRequiresPrewarmForImageFunctions(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	repoRoot := newTestRepoRoot(t)

	templatePath := filepath.Join(repoRoot, "template.yaml")
	artifactPath := writeTestArtifactManifest(t, true)

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	req := Request{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: templatePath,
		ArtifactPath: artifactPath,
		OutputDir:    ".out",
		ImagePrewarm: "off",
	}

	workflow := NewDeployWorkflow(builder.Build, envApplier.Apply, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	err := workflow.Run(req)
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

	repoRoot := newTestRepoRoot(t)

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

func TestDeployWorkflowApplyRequiresArtifactPath(t *testing.T) {
	ui := &testUI{}
	runner := &fakeComposeRunner{}
	workflow := NewDeployWorkflow(nil, nil, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	repoRoot := newTestRepoRoot(t)

	err := workflow.Apply(Request{
		Context: state.Context{
			ProjectDir:     repoRoot,
			ComposeProject: "esb-dev",
		},
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		ImagePrewarm: "all",
	})
	if err == nil || !strings.Contains(err.Error(), errArtifactPathRequired.Error()) {
		t.Fatalf("expected artifact-path-required error, got %v", err)
	}
}

func TestDeployWorkflowApplySuccess(t *testing.T) {
	ui := &testUI{}
	runner := &fakeComposeRunner{}
	workflow := NewDeployWorkflow(nil, nil, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	repoRoot := newTestRepoRoot(t)
	templatePath := filepath.Join(repoRoot, "template.yaml")

	artifactPath := writeTestArtifactManifest(t, false)
	err := workflow.Apply(Request{
		Context: state.Context{
			ComposeProject: "esb-dev",
			ProjectDir:     repoRoot,
		},
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: templatePath,
		ArtifactPath: artifactPath,
		ImagePrewarm: "all",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	configDir, err := staging.ConfigDir(templatePath, "esb-dev", "dev")
	if err != nil {
		t.Fatalf("resolve config dir: %v", err)
	}
	if got := os.Getenv(constants.EnvConfigDir); got != filepath.ToSlash(configDir) {
		t.Fatalf("CONFIG_DIR=%q, want %q", got, filepath.ToSlash(configDir))
	}
	if _, err := os.Stat(filepath.Join(configDir, "functions.yml")); err != nil {
		t.Fatalf("expected functions.yml in apply config dir: %v", err)
	}
	if len(ui.success) != 1 || !strings.Contains(ui.success[0], "Deploy complete") {
		t.Fatalf("expected deploy success message, got %#v", ui.success)
	}
}

func TestDeployWorkflowApplyRequiresTemplatePath(t *testing.T) {
	ui := &testUI{}
	runner := &fakeComposeRunner{}
	workflow := NewDeployWorkflow(nil, nil, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	repoRoot := newTestRepoRoot(t)

	artifactPath := writeTestArtifactManifest(t, false)
	err := workflow.Apply(Request{
		Context: state.Context{
			ComposeProject: "esb-dev",
			ProjectDir:     repoRoot,
		},
		Env:          "dev",
		Mode:         "docker",
		ArtifactPath: artifactPath,
		ImagePrewarm: "all",
	})
	if !errors.Is(err, errApplyTemplatePathRequired) {
		t.Fatalf("expected errApplyTemplatePathRequired, got %v", err)
	}
}

func TestDeployWorkflowApplyRequiresComposeProject(t *testing.T) {
	ui := &testUI{}
	runner := &fakeComposeRunner{}
	workflow := NewDeployWorkflow(nil, nil, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	repoRoot := newTestRepoRoot(t)

	artifactPath := writeTestArtifactManifest(t, false)
	err := workflow.Apply(Request{
		Context: state.Context{
			ProjectDir: repoRoot,
		},
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		ArtifactPath: artifactPath,
		ImagePrewarm: "all",
	})
	if !errors.Is(err, errApplyComposeProjectMissing) {
		t.Fatalf("expected errApplyComposeProjectMissing, got %v", err)
	}
}

func TestDeployWorkflowApplyRequiresEnv(t *testing.T) {
	ui := &testUI{}
	runner := &fakeComposeRunner{}
	workflow := NewDeployWorkflow(nil, nil, ui, runner)
	workflow.RegistryWaiter = noopRegistryWaiter
	repoRoot := newTestRepoRoot(t)

	artifactPath := writeTestArtifactManifest(t, false)
	err := workflow.Apply(Request{
		Context: state.Context{
			ComposeProject: "esb-dev",
			ProjectDir:     repoRoot,
		},
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		ArtifactPath: artifactPath,
		ImagePrewarm: "all",
	})
	if !errors.Is(err, errApplyEnvRequired) {
		t.Fatalf("expected errApplyEnvRequired, got %v", err)
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

	repoRoot := newTestRepoRoot(t)

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

	repoRoot := newTestRepoRoot(t)

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  provisioner: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	err := workflow.runProvisioner(
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

	repoRoot := newTestRepoRoot(t)

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

func writeTestArtifactManifest(t *testing.T, includeImageImport bool) string {
	t.Helper()
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	configDir := filepath.Join(artifactRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create artifact config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "functions.yml"), []byte("functions: {}\n"), 0o600); err != nil {
		t.Fatalf("write functions.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "routing.yml"), []byte("routes: []\n"), 0o600); err != nil {
		t.Fatalf("write routing.yml: %v", err)
	}
	if includeImageImport {
		payload := map[string]any{
			"version": "1",
			"images": []map[string]string{
				{
					"function_name": "lambda-image",
					"image_source":  "public.ecr.aws/example/repo:latest",
					"image_ref":     "registry:5010/public.ecr.aws/example/repo:latest",
				},
			},
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal image-import.json: %v", err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "image-import.json"), data, 0o600); err != nil {
			t.Fatalf("write image-import.json: %v", err)
		}
	}

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../artifact",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template.yaml",
					SHA256: "sha-template",
				},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)

	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write artifact manifest: %v", err)
	}
	return manifestPath
}

func newTestRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	setWorkingDir(t, root)
	if err := os.WriteFile(filepath.Join(root, "docker-compose.docker.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write docker compose marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "template.yaml"), []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template marker: %v", err)
	}
	return root
}

func setWorkingDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd %s: %v", wd, err)
		}
	})
}
