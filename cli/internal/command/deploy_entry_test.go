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

	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
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
	runCalls int
}

func (p *deployEntryProvisioner) CheckServicesStatus(string, string) {}

func (p *deployEntryProvisioner) RunProvisioner(
	string,
	string,
	bool,
	bool,
	string,
	[]string,
) error {
	p.runCalls++
	return nil
}

type deployEntryBuilder struct {
	requests []build.BuildRequest
}

func (b *deployEntryBuilder) Build(req build.BuildRequest) error {
	b.requests = append(b.requests, req)
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

func TestDeployCommandRunRejectsConflictingDepsFlags(t *testing.T) {
	cmd := &deployCommand{
		build:         func(build.BuildRequest) error { return nil },
		applyRuntime:  func(state.Context) error { return nil },
		ui:            deployEntryUI{},
		composeRunner: deployEntryRunner{},
	}
	err := cmd.Run(
		deployInputs{
			Templates: []deployTemplateInput{{TemplatePath: "template.yaml"}},
		},
		DeployCmd{NoDeps: true, WithDeps: true},
	)
	if err == nil {
		t.Fatalf("expected flag conflict error")
	}
}

func TestDeployCommandRunBuildsAllTemplatesAndRunsProvisionerOnlyOnLast(t *testing.T) {
	tmp := t.TempDir()
	templateA := filepath.Join(tmp, "a.template.yaml")
	templateB := filepath.Join(tmp, "b.template.yaml")
	if err := os.WriteFile(templateA, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template A: %v", err)
	}
	if err := os.WriteFile(templateB, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template B: %v", err)
	}

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
				{TemplatePath: templateA, OutputDir: ".out/a"},
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
	if provisioner.runCalls != 1 {
		t.Fatalf("expected provisioner run once for final template, got %d", provisioner.runCalls)
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
