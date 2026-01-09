// Where: cli/internal/app/down_test.go
// What: Tests for down command wiring.
// Why: Ensure down command targets the correct project.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type fakeDowner struct {
	projects      []string
	removeVolumes []bool
	err           error
}

func (f *fakeDowner) Down(project string, removeVolumes bool) error {
	f.projects = append(f.projects, project)
	f.removeVolumes = append(f.removeVolumes, removeVolumes)
	return f.err
}

func TestRunDownCallsDowner(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Downer: downer}

	exitCode := Run([]string{"down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != "esb-default" {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
	if len(downer.removeVolumes) != 1 || downer.removeVolumes[0] {
		t.Fatalf("unexpected removeVolumes: %v", downer.removeVolumes)
	}
}

func TestRunDownWithEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Downer: downer}

	exitCode := Run([]string{"--env", "staging", "down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != "esb-staging" {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
}

func TestRunDownMissingDowner(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"down"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing downer")
	}
}

func TestRunDownUsesActiveEnvFromGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configPath, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	globalCfg := config.GlobalConfig{
		Version:       1,
		ActiveProject: "demo",
		ActiveEnvironments: map[string]string{
			"demo": "staging",
		},
		Projects: map[string]config.ProjectEntry{
			"demo": {Path: projectDir},
		},
	}
	if err := config.SaveGlobalConfig(configPath, globalCfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Downer: downer}

	exitCode := Run([]string{"down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != "esb-staging" {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
}

func writeGeneratorFixture(projectDir, env string) error {
	return writeGeneratorFixtureWithMode(projectDir, env, "docker")
}

func writeGeneratorFixtureWithMode(projectDir, env, mode string) error {
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		return err
	}

	cfg := config.GeneratorConfig{
		Environments: config.Environments{{Name: env, Mode: mode}},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
	}
	return config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg)
}
