package artifactcore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_RuntimeMetaMajorMismatchAlwaysFails(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			APIVersion: "2.0",
		},
	})

	err := Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Strict:       false,
	})
	if err == nil {
		t.Fatal("expected major mismatch to fail")
	}
	if !strings.Contains(err.Error(), "major mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApply_RuntimeMetaMinorMismatchWarnsUnlessStrict(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			APIVersion: "1.1",
		},
	})

	var warnings bytes.Buffer
	err := Apply(ApplyRequest{
		ArtifactPath:  manifestPath,
		OutputDir:     filepath.Join(root, "out"),
		Strict:        false,
		WarningWriter: &warnings,
	})
	if err != nil {
		t.Fatalf("non-strict apply should pass on minor mismatch: %v", err)
	}
	if !strings.Contains(warnings.String(), "minor mismatch") {
		t.Fatalf("expected warning output, got %q", warnings.String())
	}

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out-strict"),
		Strict:       true,
	})
	if err == nil {
		t.Fatal("strict apply should fail on minor mismatch")
	}
	if !strings.Contains(err.Error(), "minor mismatch") {
		t.Fatalf("unexpected strict error: %v", err)
	}
}

func TestApply_RuntimeDigestMismatchWarnsUnlessStrict(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			PythonSitecustomizeDigest: "deadbeef",
		},
	})

	var warnings bytes.Buffer
	err := Apply(ApplyRequest{
		ArtifactPath:  manifestPath,
		OutputDir:     filepath.Join(root, "out"),
		Strict:        false,
		WarningWriter: &warnings,
	})
	if err != nil {
		t.Fatalf("non-strict apply should pass on digest mismatch: %v", err)
	}
	if !strings.Contains(warnings.String(), "python_sitecustomize_digest mismatch") {
		t.Fatalf("expected digest mismatch warning, got %q", warnings.String())
	}

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out-strict"),
		Strict:       true,
	})
	if err == nil {
		t.Fatal("strict apply should fail on digest mismatch")
	}
	if !strings.Contains(err.Error(), "python_sitecustomize_digest mismatch") {
		t.Fatalf("unexpected strict error: %v", err)
	}
}

func TestApply_RuntimeDigestVerificationMissingArtifactSourceWarnsUnlessStrict(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			PythonSitecustomizeDigest: "cafebabe",
		},
	})
	if err := os.Remove(filepath.Join(root, "fixture", artifactPythonSitecustomizeRel)); err != nil {
		t.Fatalf("remove fixture sitecustomize: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	var warnings bytes.Buffer
	err = Apply(ApplyRequest{
		ArtifactPath:  manifestPath,
		OutputDir:     filepath.Join(root, "out"),
		Strict:        false,
		WarningWriter: &warnings,
	})
	if err != nil {
		t.Fatalf("non-strict apply should pass when digest source is unavailable: %v", err)
	}
	if !strings.Contains(warnings.String(), "python_sitecustomize_digest source unreadable") {
		t.Fatalf("expected source warning, got %q", warnings.String())
	}

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out-strict"),
		Strict:       true,
	})
	if err == nil {
		t.Fatal("strict apply should fail when digest source is unavailable")
	}
	if !strings.Contains(err.Error(), "python_sitecustomize_digest source unreadable") {
		t.Fatalf("unexpected strict error: %v", err)
	}
}

func TestApply_RuntimeDigestVerificationUsesArtifactSourcesOutsideRepoRoot(t *testing.T) {
	root := t.TempDir()
	fixtureDigests := writeFixtureRuntimeMetaSources(t, filepath.Join(root, "fixture"))
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			PythonSitecustomizeDigest: fixtureDigests.pythonSitecustomize,
		},
	})

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	outside := t.TempDir()
	if err := os.Chdir(outside); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out-strict"),
		Strict:       true,
	})
	if err != nil {
		t.Fatalf("strict apply should pass with artifact-local digest verification: %v", err)
	}
}

func writeArtifactFixtureManifest(t *testing.T, root string, meta ArtifactRuntimeMeta) string {
	t.Helper()

	artifactRoot := filepath.Join(root, "fixture")
	writeFixtureRuntimeMetaSources(t, artifactRoot)
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), map[string]any{"functions": map[string]any{}})
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), map[string]any{"routes": []any{}})

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../fixture",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template.yaml",
					SHA256: "sha",
				},
				RuntimeMeta: meta,
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID("/tmp/template.yaml", nil, "sha")
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func writeFixtureRuntimeMetaSources(t *testing.T, artifactRoot string) runtimeAssetDigests {
	t.Helper()
	pythonPath := filepath.Join(artifactRoot, artifactPythonSitecustomizeRel)

	writeFixtureFile(t, pythonPath, "print('fixture sitecustomize')\n")

	pythonDigest, err := fileSHA256(pythonPath)
	if err != nil {
		t.Fatalf("hash fixture python sitecustomize: %v", err)
	}

	return runtimeAssetDigests{
		pythonSitecustomize: pythonDigest,
	}
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file %s: %v", path, err)
	}
}
