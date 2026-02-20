// Where: cli/internal/command/deploy_entry_test.go
// What: Unit tests for deploy command wiring and run branches.
// Why: Protect deploy orchestration regressions around DI and multi-template behavior.
package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

type deployEntryUI struct{}

func (deployEntryUI) Info(string) {}

func (deployEntryUI) Warn(string) {}

func (deployEntryUI) Success(string) {}

func (deployEntryUI) Block(string, string, []ui.KeyValue) {}

type deployEntryRunner struct{}

func (deployEntryRunner) Run(context.Context, string, string, ...string) error { return nil }

func (deployEntryRunner) RunOutput(context.Context, string, string, ...string) ([]byte, error) {
	return nil, nil
}

func (deployEntryRunner) RunQuiet(context.Context, string, string, ...string) error { return nil }

type deployEntryProvisioner struct {
	runCalls   int
	noDepsArgs []bool
}

func (p *deployEntryProvisioner) CheckServicesStatus(string, string) {}

func (p *deployEntryProvisioner) RunProvisioner(
	_ string,
	_ string,
	noDeps bool,
	_ bool,
	_ string,
	_ []string,
) error {
	p.runCalls++
	p.noDepsArgs = append(p.noDepsArgs, noDeps)
	return nil
}

type deployEntryBuilder struct {
	requests []build.BuildRequest
}

func (b *deployEntryBuilder) Build(req build.BuildRequest) error {
	b.requests = append(b.requests, req)
	outputRoot := domaincfg.ResolveOutputSummary(req.TemplatePath, req.OutputDir, req.Env)
	configDir := filepath.Join(outputRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(configDir, "functions.yml"), []byte("functions: {}\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(configDir, "routing.yml"), []byte("routes: []\n"), 0o600); err != nil {
		return err
	}
	return nil
}

func TestNewDeployCommandRequiresComposeRunner(t *testing.T) {
	_, err := resolveDeployCommandConfig(
		DeployDeps{
			Build: DeployBuildDeps{
				Build: func(build.BuildRequest) error { return nil },
			},
			Runtime: DeployRuntimeDeps{
				ApplyRuntimeEnv: func(state.Context) error { return nil },
			},
			Provision: DeployProvisionDeps{
				NewDeployUI: func(_ io.Writer, _ bool) ui.UserInterface { return deployEntryUI{} },
			},
		},
		&bytes.Buffer{},
		false,
	)
	if err == nil {
		t.Fatalf("expected compose runner configuration error")
	}
}

func TestResolveDeployCommandConfigRequiresRuntimeApplier(t *testing.T) {
	_, err := resolveDeployCommandConfig(
		DeployDeps{
			Build: DeployBuildDeps{
				Build: func(build.BuildRequest) error { return nil },
			},
			Provision: DeployProvisionDeps{
				ComposeRunner: deployEntryRunner{},
				NewDeployUI:   func(_ io.Writer, _ bool) ui.UserInterface { return deployEntryUI{} },
				ComposeProvisionerFactory: func(_ ui.UserInterface) ComposeProvisioner {
					return &deployEntryProvisioner{}
				},
			},
		},
		&bytes.Buffer{},
		false,
	)
	if err == nil {
		t.Fatalf("expected runtime env applier configuration error")
	}
}

func TestResolveDeployCommandConfigUsesInjectedRuntimeApplier(t *testing.T) {
	runtimeCalled := false
	config, err := resolveDeployCommandConfig(
		DeployDeps{
			Build: DeployBuildDeps{
				Build: func(build.BuildRequest) error { return nil },
			},
			Runtime: DeployRuntimeDeps{
				ApplyRuntimeEnv: func(state.Context) error {
					runtimeCalled = true
					return nil
				},
			},
			Provision: DeployProvisionDeps{
				ComposeRunner: deployEntryRunner{},
				NewDeployUI:   func(_ io.Writer, _ bool) ui.UserInterface { return deployEntryUI{} },
				ComposeProvisionerFactory: func(_ ui.UserInterface) ComposeProvisioner {
					return &deployEntryProvisioner{}
				},
			},
		},
		&bytes.Buffer{},
		false,
	)
	if err != nil {
		t.Fatalf("resolve deploy command config: %v", err)
	}
	if config.applyRuntime == nil {
		t.Fatalf("expected applyRuntime to be configured")
	}
	if err := config.applyRuntime(state.Context{}); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if !runtimeCalled {
		t.Fatalf("expected injected runtime applier to be called")
	}
}

func TestDeployCommandRunBuildsAllTemplatesAndRunsProvisionerOnlyOnLast(t *testing.T) {
	tmp := t.TempDir()
	setWorkingDir(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.docker.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templateA := filepath.Join(tmp, "a.template.yaml")
	templateB := filepath.Join(tmp, "b.template.yaml")
	if err := os.WriteFile(templateA, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template A: %v", err)
	}
	if err := os.WriteFile(templateB, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template B: %v", err)
	}
	writeTestRuntimeAssets(t, tmp)

	builder := &deployEntryBuilder{}
	provisioner := &deployEntryProvisioner{}
	cmd := &deployCommand{
		build:         builder.Build,
		applyRuntime:  func(state.Context) error { return nil },
		ui:            deployEntryUI{},
		composeRunner: deployEntryRunner{},
		workflow: deployWorkflowDeps{
			composeProvisioner: provisioner,
			registryWaiter:     func(string, time.Duration) error { return nil },
		},
	}

	err := cmd.Run(
		deployInputs{
			ProjectDir: tmp,
			Env:        "dev",
			Mode:       "docker",
			Project:    "esb-dev",
			Templates: []deployTemplateInput{
				{
					TemplatePath:  templateA,
					OutputDir:     ".out/a",
					ImageSources:  map[string]string{"image-a": "public.ecr.aws/example/a:latest"},
					ImageRuntimes: map[string]string{"image-a": "java21"},
				},
				{TemplatePath: templateB, OutputDir: ".out/b"},
			},
		},
		DeployCmd{},
	)
	if err != nil {
		t.Fatalf("run deploy command: %v", err)
	}
	if len(builder.requests) != 2 {
		t.Fatalf("expected 2 build requests, got %d", len(builder.requests))
	}
	if !builder.requests[0].BuildImages || !builder.requests[1].BuildImages {
		t.Fatalf("deploy command must build images by default: %#v", builder.requests)
	}
	if builder.requests[0].ImageRuntimes["image-a"] != "java21" {
		t.Fatalf("expected image runtime to be forwarded, got %#v", builder.requests[0].ImageRuntimes)
	}
	if builder.requests[0].ImageSources["image-a"] != "public.ecr.aws/example/a:latest" {
		t.Fatalf("expected image source to be forwarded, got %#v", builder.requests[0].ImageSources)
	}
	if provisioner.runCalls != 1 {
		t.Fatalf("expected provisioner run once for final template, got %d", provisioner.runCalls)
	}
	if len(provisioner.noDepsArgs) != 1 || !provisioner.noDepsArgs[0] {
		t.Fatalf("expected default noDeps=true, got %#v", provisioner.noDepsArgs)
	}

	manifestPath := resolveDeployArtifactManifestPath(tmp, "esb-dev", "dev")
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		t.Fatalf("read artifact manifest: %v", err)
	}
	if len(manifest.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(manifest.Artifacts))
	}
	if manifest.Artifacts[0].SourceTemplate.Path != filepath.Base(templateA) {
		t.Fatalf("unexpected first artifact template path: %s", manifest.Artifacts[0].SourceTemplate.Path)
	}
	if manifest.Artifacts[1].SourceTemplate.Path != filepath.Base(templateB) {
		t.Fatalf("unexpected second artifact template path: %s", manifest.Artifacts[1].SourceTemplate.Path)
	}
	if manifest.Artifacts[0].RuntimeConfigDir != "config" {
		t.Fatalf("unexpected runtime_config_dir: %s", manifest.Artifacts[0].RuntimeConfigDir)
	}
	if manifest.Artifacts[0].ArtifactRoot == "" || manifest.Artifacts[1].ArtifactRoot == "" {
		t.Fatalf("artifact_root must not be empty: %#v", manifest.Artifacts)
	}
}

func TestDeployCommandRunWithDepsDisablesNoDeps(t *testing.T) {
	tmp := t.TempDir()
	setWorkingDir(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.docker.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(tmp, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	writeTestRuntimeAssets(t, tmp)

	builder := &deployEntryBuilder{}
	provisioner := &deployEntryProvisioner{}
	cmd := &deployCommand{
		build:         builder.Build,
		applyRuntime:  func(state.Context) error { return nil },
		ui:            deployEntryUI{},
		composeRunner: deployEntryRunner{},
		workflow: deployWorkflowDeps{
			composeProvisioner: provisioner,
			registryWaiter:     func(string, time.Duration) error { return nil },
		},
	}

	err := cmd.Run(
		deployInputs{
			ProjectDir: tmp,
			Env:        "dev",
			Mode:       "docker",
			Project:    "esb-dev",
			Templates:  []deployTemplateInput{{TemplatePath: templatePath}},
		},
		DeployCmd{WithDeps: true},
	)
	if err != nil {
		t.Fatalf("run deploy command: %v", err)
	}
	if len(provisioner.noDepsArgs) != 1 || provisioner.noDepsArgs[0] {
		t.Fatalf("expected noDeps=false when --with-deps is enabled, got %#v", provisioner.noDepsArgs)
	}
}

func TestDeployCommandRunAllowsRenderOnlyGenerate(t *testing.T) {
	tmp := t.TempDir()
	setWorkingDir(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.docker.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(tmp, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	writeTestRuntimeAssets(t, tmp)

	builder := &deployEntryBuilder{}
	provisioner := &deployEntryProvisioner{}
	cmd := &deployCommand{
		build:         builder.Build,
		applyRuntime:  func(state.Context) error { return nil },
		ui:            deployEntryUI{},
		composeRunner: deployEntryRunner{},
		workflow: deployWorkflowDeps{
			composeProvisioner: provisioner,
			registryWaiter:     func(string, time.Duration) error { return nil },
		},
	}

	err := cmd.runWithOverrides(
		deployInputs{
			ProjectDir: tmp,
			Env:        "dev",
			Mode:       "docker",
			Project:    "esb-dev",
			Templates:  []deployTemplateInput{{TemplatePath: templatePath}},
		},
		DeployCmd{BuildOnly: true, SecretEnv: "secret.env"},
		deployRunOverrides{
			buildImages:    boolPtr(false),
			forceBuildOnly: true,
		},
	)
	if err != nil {
		t.Fatalf("run deploy command: %v", err)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	if builder.requests[0].BuildImages {
		t.Fatalf("expected render-only generate request, got %#v", builder.requests[0])
	}
	if provisioner.runCalls != 0 {
		t.Fatalf("provisioner must not run when BuildOnly=true, got %d", provisioner.runCalls)
	}
}

func writeTestRuntimeAssets(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		filepath.Join("runtime-hooks", "python", "sitecustomize", "site-packages", "sitecustomize.py"): "print('ok')\n",
		filepath.Join("runtime-hooks", "java", "agent", "lambda-java-agent.jar"):                       "jar-agent",
		filepath.Join("runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar"):                   "jar-wrapper",
		filepath.Join("cli", "assets", "runtime-templates", "java", "templates", "dockerfile.tmpl"):    "FROM eclipse-temurin:21\n",
		filepath.Join("cli", "assets", "runtime-templates", "python", "templates", "dockerfile.tmpl"):  "FROM python:3.12\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func TestRunDeployRejectsConflictingEmojiFlags(t *testing.T) {
	var out bytes.Buffer
	exitCode := runDeploy(
		CLI{Deploy: DeployCmd{Emoji: true, NoEmoji: true}},
		Dependencies{},
		&out,
	)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), "--emoji and --no-emoji cannot be used together") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunDeployRequiresBuilder(t *testing.T) {
	var out bytes.Buffer
	exitCode := runDeploy(CLI{Deploy: DeployCmd{}}, Dependencies{}, &out)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), errDeployBuilderNotConfigured.Error()) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunDeployHandlesInputResolutionError(t *testing.T) {
	var out bytes.Buffer
	exitCode := runDeploy(
		CLI{Deploy: DeployCmd{}},
		Dependencies{
			RepoResolver: func(string) (string, error) {
				return "", errors.New("repo fail")
			},
			Deploy: DeployDeps{
				Build: DeployBuildDeps{
					Build: func(build.BuildRequest) error { return nil },
				},
				Runtime: DeployRuntimeDeps{
					ApplyRuntimeEnv: func(state.Context) error { return nil },
				},
				Provision: DeployProvisionDeps{
					ComposeRunner: deployEntryRunner{},
					NewDeployUI:   func(_ io.Writer, _ bool) ui.UserInterface { return deployEntryUI{} },
					ComposeProvisionerFactory: func(_ ui.UserInterface) ComposeProvisioner {
						return &deployEntryProvisioner{}
					},
				},
			},
		},
		&out,
	)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), "resolve repo root: repo fail") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestNewDeployCommandCopiesConfig(t *testing.T) {
	buildFn := func(build.BuildRequest) error { return nil }
	applyRuntime := func(state.Context) error { return nil }
	runner := deployEntryRunner{}
	config := deployCommandConfig{
		build:         buildFn,
		applyRuntime:  applyRuntime,
		ui:            deployEntryUI{},
		composeRunner: runner,
		workflow: deployWorkflowDeps{
			registryWaiter: func(string, time.Duration) error { return nil },
		},
		emojiEnabled: true,
	}

	cmd := newDeployCommand(config)
	if reflect.ValueOf(cmd.build).Pointer() != reflect.ValueOf(buildFn).Pointer() {
		t.Fatal("expected build function to match config")
	}
	if reflect.ValueOf(cmd.applyRuntime).Pointer() != reflect.ValueOf(applyRuntime).Pointer() {
		t.Fatal("expected applyRuntime function to match config")
	}
	if !cmd.emojiEnabled {
		t.Fatal("expected emojiEnabled=true")
	}
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
