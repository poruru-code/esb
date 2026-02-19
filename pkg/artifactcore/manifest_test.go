package artifactcore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRoundTripAndValidateID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:       "/tmp/template-a.yaml",
					SHA256:     "sha-a",
					Parameters: map[string]string{"Stage": "dev"},
				},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)
	if err := WriteArtifactManifest(path, manifest); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}
	if _, err := ReadArtifactManifest(path); err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
	}
}

func TestComputeArtifactIDDeterministic(t *testing.T) {
	first := ComputeArtifactID("./svc/../template.yaml", map[string]string{"B": "2", "A": "1"}, "sha")
	second := ComputeArtifactID("template.yaml", map[string]string{"A": "1", "B": "2"}, "sha")
	if first != second {
		t.Fatalf("id mismatch: %s != %s", first, second)
	}
	if !strings.HasPrefix(first, "template-") {
		t.Fatalf("unexpected id: %s", first)
	}
}

func TestManifestValidateRejectsUnsupportedSchemaVersion(t *testing.T) {
	manifest := ArtifactManifest{
		SchemaVersion: "2",
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
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

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected unsupported schema_version to fail validation")
	}
	if !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadArtifactManifestAllowsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	id := ComputeArtifactID("/tmp/template-a.yaml", map[string]string{"Stage": "dev"}, "sha-a")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`future_top_level: true`,
		`artifacts:`,
		`  - id: ` + id,
		`    artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    future_entry_field: abc`,
		`    source_template:`,
		`      path: /tmp/template-a.yaml`,
		`      sha256: sha-a`,
		`      parameters:`,
		`        Stage: dev`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := ReadArtifactManifest(path); err != nil {
		t.Fatalf("ReadArtifactManifest() should accept unknown fields: %v", err)
	}
}

func TestReadArtifactManifestMissingFile(t *testing.T) {
	_, err := ReadArtifactManifest(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("expected missing-file error")
	}
	var missingPathErr MissingReferencedPathError
	if !errors.As(err, &missingPathErr) {
		t.Fatalf("expected MissingReferencedPathError, got: %v", err)
	}
	if !strings.HasSuffix(missingPathErr.Path, "missing.yml") {
		t.Fatalf("unexpected missing path: %q", missingPathErr.Path)
	}
}

func TestResolveBundleManifest(t *testing.T) {
	manifest := ArtifactManifest{
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:   "./artifact-a",
				BundleManifest: "bundle/manifest.json",
			},
		},
	}
	manifestPath := filepath.Join("/tmp", "project", "artifact.yml")

	got, err := manifest.ResolveBundleManifest(manifestPath, 0)
	if err != nil {
		t.Fatalf("ResolveBundleManifest() error = %v", err)
	}
	want := filepath.Join("/tmp", "project", "artifact-a", "bundle", "manifest.json")
	if got != want {
		t.Fatalf("bundle path = %q, want %q", got, want)
	}
}

func TestResolveBundleManifestReturnsEmptyWhenUnset(t *testing.T) {
	manifest := ArtifactManifest{
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot: "./artifact-a",
			},
		},
	}
	got, err := manifest.ResolveBundleManifest(filepath.Join("/tmp", "project", "artifact.yml"), 0)
	if err != nil {
		t.Fatalf("ResolveBundleManifest() error = %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty bundle path, got %q", got)
	}
}

func TestResolveBundleManifestRejectsEscapingPath(t *testing.T) {
	manifest := ArtifactManifest{
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:   "./artifact-a",
				BundleManifest: "../escape.json",
			},
		},
	}
	_, err := manifest.ResolveBundleManifest(filepath.Join("/tmp", "project", "artifact.yml"), 0)
	if err == nil {
		t.Fatal("expected error for escaping bundle path")
	}
	if !strings.Contains(err.Error(), "must not escape artifact root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteArtifactManifestFailsWhenParentIsFile(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}
	manifest := validTestManifest()

	err := WriteArtifactManifest(filepath.Join(blocked, "artifact.yml"), manifest)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create artifact manifest directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteArtifactManifestFailsOnInvalidManifest(t *testing.T) {
	manifest := ArtifactManifest{
		SchemaVersion: "",
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ID:               "template-aaaaaaaa",
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path: "/tmp/template-a.yaml",
				},
			},
		},
	}

	err := WriteArtifactManifest(filepath.Join(t.TempDir(), "artifact.yml"), manifest)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "schema_version is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validTestManifest() ArtifactManifest {
	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:       "/tmp/template-a.yaml",
					SHA256:     "sha-a",
					Parameters: map[string]string{"Stage": "dev"},
				},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)
	return manifest
}
