package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
	"gopkg.in/yaml.v3"
)

func TestApplyArtifactRuntimeConfigUsesArtifactManifest(t *testing.T) {
	root := t.TempDir()

	artifactRoot := filepath.Join(root, "artifact-a")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), "functions:\n  hello:\n    handler: a.handler\n")
	writeRuntimeConfigFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), "routes:\n  - path: /hello\n    method: GET\n    function: hello\n")

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []artifactcore.ArtifactEntry{
			{
				ArtifactRoot:     "../artifact-a",
				RuntimeConfigDir: "config",
				SourceTemplate: artifactcore.ArtifactSourceTemplate{
					Path:   "/tmp/template-a.yaml",
					SHA256: "sha-a",
				},
			},
		},
	}
	manifest.Artifacts[0].ID = artifactcore.ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
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
	if !errors.Is(err, artifactcore.ErrArtifactPathRequired) {
		t.Fatalf("expected ErrArtifactPathRequired, got %v", err)
	}
}

func TestApplyArtifactRuntimeConfigWrapsCoreErrors(t *testing.T) {
	workflow := Workflow{}
	err := workflow.applyArtifactRuntimeConfig(Request{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error when artifact path is empty")
	}
	if err.Error() == artifactcore.ErrArtifactPathRequired.Error() {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if !errors.Is(err, artifactcore.ErrArtifactPathRequired) {
		t.Fatalf("expected ErrArtifactPathRequired, got %v", err)
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
