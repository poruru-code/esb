package command

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	engine "github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestArtifactGenerateToDeployFlags(t *testing.T) {
	cmd := ArtifactGenerateCmd{
		Mode:         "docker",
		Output:       ".out",
		Manifest:     ".out/artifact.yml",
		Project:      "esb-dev",
		ComposeFiles: []string{"docker-compose.yml"},
		ImageURI:     []string{"fn=image:latest"},
		ImageRuntime: []string{"fn=python"},
		Bundle:       true,
		BuildImages:  true,
		NoCache:      true,
		Verbose:      true,
		Emoji:        true,
		Force:        true,
		NoSave:       true,
	}
	got := artifactGenerateToDeployFlags(cmd)
	if !got.BuildOnly {
		t.Fatal("BuildOnly must be true for artifact generate")
	}
	if got.Mode != cmd.Mode || got.Output != cmd.Output || got.Manifest != cmd.Manifest || got.Project != cmd.Project {
		t.Fatalf("basic flag mapping mismatch: %#v", got)
	}
	if !reflect.DeepEqual(got.ComposeFiles, cmd.ComposeFiles) {
		t.Fatalf("compose files mismatch: got=%v want=%v", got.ComposeFiles, cmd.ComposeFiles)
	}
	if !reflect.DeepEqual(got.ImageURI, cmd.ImageURI) {
		t.Fatalf("image uri mismatch: got=%v want=%v", got.ImageURI, cmd.ImageURI)
	}
	if !reflect.DeepEqual(got.ImageRuntime, cmd.ImageRuntime) {
		t.Fatalf("image runtime mismatch: got=%v want=%v", got.ImageRuntime, cmd.ImageRuntime)
	}
	if !got.Bundle || !got.NoCache || !got.Verbose || !got.Emoji || !got.Force || !got.NoSave {
		t.Fatalf("boolean/metadata mapping mismatch: %#v", got)
	}
}

func TestArtifactGenerateToDeployFlagsDefaultsToRenderOnly(t *testing.T) {
	got := artifactGenerateToDeployFlags(ArtifactGenerateCmd{})
	if !got.BuildOnly {
		t.Fatal("BuildOnly must be true for artifact generate")
	}
}

func TestRunArtifactGenerateDelegatesToDeploy(t *testing.T) {
	var out bytes.Buffer
	exitCode := runArtifactGenerate(CLI{}, Dependencies{}, &out)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), "builder not configured") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunArtifactGenerateUsesRenderOnlyOverrides(t *testing.T) {
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
	deps := Dependencies{
		RepoResolver: func(string) (string, error) { return tmp, nil },
		Deploy: DeployDeps{
			Build: DeployBuildDeps{
				Build: builder.Build,
			},
			Runtime: DeployRuntimeDeps{
				ApplyRuntimeEnv: func(state.Context) error { return nil },
			},
			Provision: DeployProvisionDeps{
				ComposeRunner: deployEntryRunner{},
				NewDeployUI: func(_ io.Writer, _ bool) ui.UserInterface {
					return deployEntryUI{}
				},
				ComposeProvisionerFactory: func(_ ui.UserInterface) ComposeProvisioner {
					return provisioner
				},
			},
		},
	}
	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Artifact: ArtifactCmd{
			Generate: ArtifactGenerateCmd{
				Mode: "docker",
			},
		},
	}

	var out bytes.Buffer
	exitCode := runArtifactGenerate(cli, deps, &out)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d output=%q", exitCode, out.String())
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected 1 build request, got %d", len(builder.requests))
	}
	if builder.requests[0].BuildImages {
		t.Fatalf("artifact generate default must be render-only, got %#v", builder.requests[0])
	}
	if provisioner.runCalls != 0 {
		t.Fatalf("provisioner must not run for artifact generate, got %d", provisioner.runCalls)
	}
}

func TestRunArtifactApplyRequiresArgs(t *testing.T) {
	var out bytes.Buffer
	exitCode := runArtifactApply(CLI{}, Dependencies{}, &out)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestRunArtifactApplySuccess(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "a")
	writeYAML(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions: {}\n")
	writeYAML(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes: []\n")

	manifest := engine.ArtifactManifest{
		SchemaVersion: engine.ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []engine.ArtifactEntry{
			{
				ArtifactRoot:     "../a",
				RuntimeConfigDir: "config",
				SourceTemplate: engine.ArtifactSourceTemplate{
					Path:   "/tmp/template.yaml",
					SHA256: "sha",
				},
			},
		},
	}
	manifest.Artifacts[0].ID = engine.ComputeArtifactID("/tmp/template.yaml", nil, "sha")
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := engine.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	outDir := filepath.Join(root, "out")
	var out bytes.Buffer
	exitCode := runArtifactApply(
		CLI{Artifact: ArtifactCmd{Apply: ArtifactApplyCmd{Artifact: manifestPath, OutputDir: outDir}}},
		Dependencies{},
		&out,
	)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d output=%q", exitCode, out.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "functions.yml")); err != nil {
		t.Fatalf("functions.yml not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "routing.yml")); err != nil {
		t.Fatalf("routing.yml not generated: %v", err)
	}
}

func TestRunArtifactApplyStrictFailsOnRuntimeMetaMismatch(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "a")
	writeYAML(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions: {}\n")
	writeYAML(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes: []\n")

	manifest := engine.ArtifactManifest{
		SchemaVersion: engine.ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []engine.ArtifactEntry{
			{
				ArtifactRoot:     "../a",
				RuntimeConfigDir: "config",
				SourceTemplate: engine.ArtifactSourceTemplate{
					Path:   "/tmp/template.yaml",
					SHA256: "sha",
				},
				RuntimeMeta: engine.ArtifactRuntimeMeta{
					Hooks: engine.RuntimeHooksMeta{
						APIVersion: "1.1",
					},
				},
			},
		},
	}
	manifest.Artifacts[0].ID = engine.ComputeArtifactID("/tmp/template.yaml", nil, "sha")
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := engine.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	outDir := filepath.Join(root, "out")
	var out bytes.Buffer
	exitCode := runArtifactApply(
		CLI{Artifact: ArtifactCmd{Apply: ArtifactApplyCmd{Artifact: manifestPath, OutputDir: outDir, Strict: true}}},
		Dependencies{},
		&out,
	)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for strict mismatch, output=%q", out.String())
	}
	if !strings.Contains(out.String(), "minor mismatch") {
		t.Fatalf("expected minor mismatch error, got %q", out.String())
	}
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
