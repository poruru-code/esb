// Where: cli/internal/app/down_test.go
// What: Tests for down command wiring.
// Why: Ensure down command targets the correct project.
package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
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
	setupProjectConfig(t, projectDir, "demo")

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Down: DownDeps{Downer: downer}}

	exitCode := Run([]string{"down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject(defaultTestAppName, "default") {
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
	setupProjectConfig(t, projectDir, "demo")

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Down: DownDeps{Downer: downer}}

	exitCode := Run([]string{"--env", "staging", "down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
}

func TestRunDownMissingDowner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_ENV", "staging")

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Down: DownDeps{Downer: downer}}

	exitCode := Run([]string{"down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected project: %v", downer.projects)
	}
}

func TestRunDownOutputsLegacySuccess(t *testing.T) {
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixEnv), "default")
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	downer := &fakeDowner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Down: DownDeps{Downer: downer}}

	exitCode := Run([]string{"down"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if out.String() != "down complete\n" {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func expectedComposeProject(appName, env string) string {
	return fmt.Sprintf("%s-%s", appName, env)
}

const defaultTestAppName = "demo"

func writeGeneratorFixture(projectDir, env string) error {
	return writeGeneratorFixtureWithMode(projectDir, env, "docker")
}

func writeGeneratorFixtureWithMode(projectDir, env, mode string) error {
	return writeGeneratorFixtureFull(projectDir, env, mode, defaultTestAppName)
}

func writeGeneratorFixtureFull(projectDir, env, mode, appName string) error {
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		return err
	}

	cfg := config.GeneratorConfig{
		App: config.AppConfig{
			Name: appName,
		},
		Environments: config.Environments{{Name: env, Mode: mode}},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
	}
	return config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg)
}
