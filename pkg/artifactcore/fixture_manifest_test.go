package artifactcore

import (
	"path/filepath"
	"strings"
	"testing"
)

func writeArtifactFixtureManifest(t *testing.T, root string) string {
	t.Helper()

	artifactRoot := filepath.Join(root, "fixture")
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

func containsWarning(warnings []string, pattern string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, pattern) {
			return true
		}
	}
	return false
}
