package artifactcore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteApplyValidatesRequiredFields(t *testing.T) {
	_, err := ExecuteApply(ApplyInput{
		ArtifactPath: "",
		OutputDir:    "out",
	})
	if !errors.Is(err, ErrArtifactPathRequired) {
		t.Fatalf("expected ErrArtifactPathRequired, got %v", err)
	}

	_, err = ExecuteApply(ApplyInput{
		ArtifactPath: "artifact.yml",
		OutputDir:    "",
	})
	if !errors.Is(err, ErrOutputDirRequired) {
		t.Fatalf("expected ErrOutputDirRequired, got %v", err)
	}
}

func TestExecuteApplyNormalizesAndApplies(t *testing.T) {
	manifestPath := writeExecutableArtifact(t)
	outputDir := filepath.Join(t.TempDir(), "out")

	if _, err := ExecuteApply(ApplyInput{
		ArtifactPath: "  " + manifestPath + "  ",
		OutputDir:    "  " + outputDir + "  ",
	}); err != nil {
		t.Fatalf("ExecuteApply() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "functions.yml")); err != nil {
		t.Fatalf("functions.yml not merged: %v", err)
	}
}

func TestExecuteApplyReturnsWarningsAsData(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root)
	setRuntimeStackRequirements(t, manifestPath, RuntimeStackMeta{
		APIVersion: "1.1",
		Mode:       "docker",
		ESBVersion: "latest",
	})

	result, err := ExecuteApply(ApplyInput{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runtime: &RuntimeObservation{
			Mode:       "docker",
			ESBVersion: "latest",
			Source:     "test",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteApply() error = %v", err)
	}
	if !containsWarning(result.Warnings, "runtime_stack.api_version minor mismatch") {
		t.Fatalf("expected runtime_stack api minor mismatch warning, got %#v", result.Warnings)
	}
}

func writeExecutableArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	runtimeDir := filepath.Join(artifactRoot, "config")
	writeRuntimeFile(t, filepath.Join(runtimeDir, "functions.yml"), "functions: {}\n")
	writeRuntimeFile(t, filepath.Join(runtimeDir, "routing.yml"), "routes: []\n")
	writeRuntimeFile(t, filepath.Join(runtimeDir, "resources.yml"), "resources: {}\n")

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ID:               ComputeArtifactID("template.yaml", nil, "sha"),
				ArtifactRoot:     "artifact",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "template.yaml",
					SHA256: "sha",
				},
			},
		},
	}
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}
	return manifestPath
}

func writeRuntimeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
