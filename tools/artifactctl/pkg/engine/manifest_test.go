package engine

import (
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
