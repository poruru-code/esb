// Where: cli/internal/workflows/env_test.go
// What: Unit tests for environment workflows.
// Why: Validate env list/add/use/remove behavior without CLI adapters.
package workflows

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type stubDetector struct {
	state state.State
	err   error
}

func (d stubDetector) Detect() (state.State, error) {
	return d.state, d.err
}

func TestEnvListWorkflowDetectsStatus(t *testing.T) {
	cfg := config.GeneratorConfig{
		App: config.AppConfig{LastEnv: "dev"},
		Environments: config.Environments{
			{Name: "dev", Mode: "docker"},
			{Name: "test", Mode: "containerd"},
		},
	}
	factory := func(_, env string) (ports.StateDetector, error) {
		switch env {
		case "dev":
			return stubDetector{state: state.StateRunning}, nil
		case "test":
			return stubDetector{state: state.StateStopped}, nil
		default:
			return stubDetector{state: state.StateInitialized}, nil
		}
	}

	result, err := NewEnvListWorkflow(factory).Run(EnvListRequest{
		ProjectDir: "/repo",
		Generator:  cfg,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Environments) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(result.Environments))
	}

	byName := map[string]EnvInfo{}
	for _, env := range result.Environments {
		byName[env.Name] = env
	}

	dev := byName["dev"]
	if !dev.Active || dev.Status != string(state.StateRunning) {
		t.Fatalf("unexpected dev status: %+v", dev)
	}
	test := byName["test"]
	if test.Active || test.Status != string(state.StateStopped) {
		t.Fatalf("unexpected test status: %+v", test)
	}
}

func TestEnvAddWorkflowWritesGeneratorConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "generator.yml")

	cfg := config.GeneratorConfig{
		App:          config.AppConfig{Name: "demo"},
		Environments: config.Environments{{Name: "dev", Mode: "docker"}},
		Paths:        config.PathsConfig{SamTemplate: "template.yaml", OutputDir: ".esb/"},
	}
	if err := config.SaveGeneratorConfig(path, cfg); err != nil {
		t.Fatalf("save generator: %v", err)
	}

	workflow := NewEnvAddWorkflow()
	if err := workflow.Run(EnvAddRequest{
		GeneratorPath: path,
		Generator:     cfg,
		Name:          "prod",
		Mode:          "containerd",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := config.LoadGeneratorConfig(path)
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	if !updated.Environments.Has("prod") {
		t.Fatalf("expected prod environment to be added")
	}
}

func TestEnvAddWorkflowRejectsDuplicate(t *testing.T) {
	cfg := config.GeneratorConfig{
		Environments: config.Environments{{Name: "dev", Mode: "docker"}},
	}
	err := NewEnvAddWorkflow().Run(EnvAddRequest{
		GeneratorPath: "/tmp/generator.yml",
		Generator:     cfg,
		Name:          "dev",
		Mode:          "docker",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestEnvUseWorkflowUpdatesGeneratorAndGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generator.yml")
	cfg := config.GeneratorConfig{
		App:          config.AppConfig{Name: "demo"},
		Environments: config.Environments{{Name: "dev", Mode: "docker"}},
		Paths:        config.PathsConfig{SamTemplate: "template.yaml", OutputDir: ".esb/"},
	}
	if err := config.SaveGeneratorConfig(genPath, cfg); err != nil {
		t.Fatalf("save generator: %v", err)
	}

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	globalCfg := config.GlobalConfig{
		Version:  1,
		Projects: map[string]config.ProjectEntry{"demo": {}},
	}
	if err := config.SaveGlobalConfig(globalPath, globalCfg); err != nil {
		t.Fatalf("save global: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := NewEnvUseWorkflow().Run(EnvUseRequest{
		EnvName:          "dev",
		ProjectName:      "demo",
		ProjectDir:       dir,
		GeneratorPath:    genPath,
		Generator:        cfg,
		GlobalConfig:     globalCfg,
		GlobalConfigPath: globalPath,
		Now:              now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.EnvName != "dev" || result.ProjectName != "demo" {
		t.Fatalf("unexpected result: %+v", result)
	}

	updatedGen, err := config.LoadGeneratorConfig(genPath)
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	if updatedGen.App.LastEnv != "dev" {
		t.Fatalf("expected LastEnv to be updated")
	}

	updatedGlobal, err := config.LoadGlobalConfig(globalPath)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	entry := updatedGlobal.Projects["demo"]
	if entry.Path != dir {
		t.Fatalf("expected project path to be updated")
	}
	if entry.LastUsed != now.Format(time.RFC3339) {
		t.Fatalf("expected last used timestamp to be updated")
	}
}

func TestEnvRemoveWorkflowRemovesEnvironment(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generator.yml")
	cfg := config.GeneratorConfig{
		App: config.AppConfig{Name: "demo", LastEnv: "dev"},
		Environments: config.Environments{
			{Name: "dev", Mode: "docker"},
			{Name: "prod", Mode: "containerd"},
		},
		Paths: config.PathsConfig{SamTemplate: "template.yaml", OutputDir: ".esb/"},
	}
	if err := config.SaveGeneratorConfig(genPath, cfg); err != nil {
		t.Fatalf("save generator: %v", err)
	}

	if err := NewEnvRemoveWorkflow().Run(EnvRemoveRequest{
		Name:          "dev",
		GeneratorPath: genPath,
		Generator:     cfg,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := config.LoadGeneratorConfig(genPath)
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	if updated.Environments.Has("dev") {
		t.Fatalf("expected dev environment to be removed")
	}
	if updated.App.LastEnv != "" {
		t.Fatalf("expected LastEnv to be cleared")
	}
}

func TestEnvRemoveWorkflowErrors(t *testing.T) {
	cfg := config.GeneratorConfig{
		Environments: config.Environments{{Name: "dev", Mode: "docker"}},
	}
	if err := NewEnvRemoveWorkflow().Run(EnvRemoveRequest{
		Name:      "missing",
		Generator: cfg,
	}); err != ErrEnvNotFound {
		t.Fatalf("expected ErrEnvNotFound, got %v", err)
	}
	if err := NewEnvRemoveWorkflow().Run(EnvRemoveRequest{
		Name:      "dev",
		Generator: cfg,
	}); err != ErrEnvLast {
		t.Fatalf("expected ErrEnvLast, got %v", err)
	}
}

func TestEnvUseWorkflowRequiresGlobalConfigPath(t *testing.T) {
	_, err := NewEnvUseWorkflow().Run(EnvUseRequest{EnvName: "dev"})
	if err == nil || !strings.Contains(err.Error(), "global config path not available") {
		t.Fatalf("expected global config path error, got %v", err)
	}
}

func TestEnvAddWorkflowRequiresNameAndMode(t *testing.T) {
	workflow := NewEnvAddWorkflow()
	if err := workflow.Run(EnvAddRequest{}); err == nil {
		t.Fatalf("expected name error")
	}
	if err := workflow.Run(EnvAddRequest{Name: "dev"}); err == nil {
		t.Fatalf("expected mode error")
	}
}
