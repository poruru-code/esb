package artifactcore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_RuntimeStackWarnsWhenObservationMissing(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root)
	setRuntimeStackRequirements(t, manifestPath, RuntimeStackMeta{
		APIVersion: RuntimeStackAPIVersion,
		Mode:       "docker",
		ESBVersion: "latest",
	})

	result, err := ExecuteApply(ApplyInput{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
	})
	if err != nil {
		t.Fatalf("apply should warn when runtime observation is missing: %v", err)
	}
	if !containsWarning(result.Warnings, "runtime stack observation is required") {
		t.Fatalf("expected runtime observation warning, got %#v", result.Warnings)
	}
}

func TestApply_RuntimeStackModeMismatchAlwaysFails(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root)
	setRuntimeStackRequirements(t, manifestPath, RuntimeStackMeta{
		APIVersion: RuntimeStackAPIVersion,
		Mode:       "containerd",
		ESBVersion: "latest",
	})

	_, err := ExecuteApply(ApplyInput{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runtime: &RuntimeObservation{
			Mode:       "docker",
			ESBVersion: "latest",
			Source:     "test",
		},
	})
	if err == nil {
		t.Fatal("expected mode mismatch to fail")
	}
	if !strings.Contains(err.Error(), "runtime_stack.mode mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApply_RuntimeStackVersionMismatchWarns(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root)
	setRuntimeStackRequirements(t, manifestPath, RuntimeStackMeta{
		APIVersion: RuntimeStackAPIVersion,
		Mode:       "docker",
		ESBVersion: "v1.2.3",
	})

	result, err := ExecuteApply(ApplyInput{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runtime: &RuntimeObservation{
			Mode:       "docker",
			ESBVersion: "v1.2.4",
			Source:     "test",
		},
	})
	if err != nil {
		t.Fatalf("apply should warn on esb version mismatch: %v", err)
	}
	if !containsWarning(result.Warnings, "runtime_stack.esb_version mismatch") {
		t.Fatalf("expected version mismatch warning, got %#v", result.Warnings)
	}
}

func TestApply_RuntimeStackAPIMinorMismatchWarns(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root)
	setRuntimeStackRequirements(t, manifestPath, RuntimeStackMeta{
		APIVersion: "1.1",
		Mode:       "docker",
		ESBVersion: "v1.2.3",
	})

	result, err := ExecuteApply(ApplyInput{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runtime: &RuntimeObservation{
			Mode:       "docker",
			ESBVersion: "v1.2.3",
			Source:     "test",
		},
	})
	if err != nil {
		t.Fatalf("apply should warn on runtime_stack api minor mismatch: %v", err)
	}
	if !containsWarning(result.Warnings, "runtime_stack.api_version minor mismatch") {
		t.Fatalf("expected minor mismatch warning, got %#v", result.Warnings)
	}
}

func setRuntimeStackRequirements(t *testing.T, manifestPath string, runtime RuntimeStackMeta) {
	t.Helper()
	manifest, err := ReadArtifactManifest(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest.RuntimeStack = runtime
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest with runtime stack: %v", err)
	}
}
