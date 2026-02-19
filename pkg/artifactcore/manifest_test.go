package artifactcore

import (
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
	if err := ValidateIDs(path); err != nil {
		t.Fatalf("ValidateIDs() error = %v", err)
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
