package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApplyArtifactRuntimeConfigUsesArtifactManifest(t *testing.T) {
	root := t.TempDir()

	artifactRoot := filepath.Join(root, "artifact-a")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions:\n  hello:\n    handler: a.handler\n")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes:\n  - path: /hello\n    method: GET\n    function: hello\n")

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../artifact-a",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template-a.yaml",
					SHA256: "sha-a",
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

	stagingDir := filepath.Join(root, "staging", "config")
	workflow := Workflow{}
	if err := workflow.applyArtifactRuntimeConfig(Request{ArtifactPath: manifestPath}, stagingDir); err != nil {
		t.Fatalf("applyArtifactRuntimeConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(stagingDir, "functions.yml"))
	if err != nil {
		t.Fatalf("read merged functions.yml: %v", err)
	}
	decoded := map[string]any{}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal functions.yml: %v", err)
	}
	functions, _ := decoded["functions"].(map[string]any)
	if functions == nil {
		t.Fatalf("functions section missing: %#v", decoded)
	}
	hello, _ := functions["hello"].(map[string]any)
	if hello == nil || hello["handler"] != "a.handler" {
		t.Fatalf("hello function mismatch: %#v", functions["hello"])
	}
}

func TestApplyArtifactRuntimeConfigFailsWhenArtifactPathEmpty(t *testing.T) {
	workflow := Workflow{}
	err := workflow.applyArtifactRuntimeConfig(Request{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error when artifact path is empty")
	}
	if err != errArtifactPathRequired {
		t.Fatalf("expected errArtifactPathRequired, got %v", err)
	}
}

func TestRuntimeProvisionApplyRequestDefaults(t *testing.T) {
	req := runtimeProvisionApplyRequest("artifact.yml", "staging-config")

	if req.ArtifactPath != "artifact.yml" {
		t.Fatalf("ArtifactPath=%q", req.ArtifactPath)
	}
	if req.OutputDir != "staging-config" {
		t.Fatalf("OutputDir=%q", req.OutputDir)
	}
	if req.SecretEnvPath != "" {
		t.Fatalf("SecretEnvPath=%q, want empty", req.SecretEnvPath)
	}
	if req.Strict {
		t.Fatal("Strict=true, want false")
	}
	if req.WarningWriter != nil {
		t.Fatalf("WarningWriter=%v, want nil", req.WarningWriter)
	}
}

func writeRuntimeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
