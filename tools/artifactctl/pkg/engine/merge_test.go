package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMergeRuntimeConfig_UsesManifestOrderAndLastWriteWins(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	aRoot := filepath.Join(root, "a")
	bRoot := filepath.Join(root, "b")
	writeYAMLFile(t, filepath.Join(aRoot, "config", "functions.yml"), map[string]any{
		"functions": map[string]any{
			"hello": map[string]any{"handler": "a.handler"},
		},
		"defaults": map[string]any{
			"environment": map[string]any{"LOG_LEVEL": "INFO"},
		},
	})
	writeYAMLFile(t, filepath.Join(aRoot, "config", "routing.yml"), map[string]any{
		"routes": []any{map[string]any{"path": "/hello", "method": "GET", "function": "hello"}},
	})
	writeYAMLFile(t, filepath.Join(aRoot, "config", "resources.yml"), map[string]any{
		"resources": map[string]any{"s3": []any{map[string]any{"BucketName": "bucket-a"}}},
	})

	writeYAMLFile(t, filepath.Join(bRoot, "config", "functions.yml"), map[string]any{
		"functions": map[string]any{
			"hello": map[string]any{"handler": "b.handler"},
			"bye":   map[string]any{"handler": "bye.handler"},
		},
		"defaults": map[string]any{
			"environment": map[string]any{"LOG_LEVEL": "DEBUG", "TRACE": "1"},
		},
	})
	writeYAMLFile(t, filepath.Join(bRoot, "config", "routing.yml"), map[string]any{
		"routes": []any{map[string]any{"path": "/hello", "method": "GET", "function": "bye"}},
	})
	writeJSONFile(t, filepath.Join(bRoot, "config", "image-import.json"), map[string]any{
		"version": "1",
		"images":  []any{map[string]any{"function_name": "img-fn", "image_ref": "repo:tag"}},
	})

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../a",
				RuntimeConfigDir: "config",
				SourceTemplate:   ArtifactSourceTemplate{Path: "/tmp/a.yaml", SHA256: "sha-a"},
			},
			{
				ArtifactRoot:     "../b",
				RuntimeConfigDir: "config",
				SourceTemplate:   ArtifactSourceTemplate{Path: "/tmp/b.yaml", SHA256: "sha-b"},
			},
		},
	}
	for i := range manifest.Artifacts {
		e := manifest.Artifacts[i]
		e.ID = ComputeArtifactID(e.SourceTemplate.Path, e.SourceTemplate.Parameters, e.SourceTemplate.SHA256)
		manifest.Artifacts[i] = e
	}
	manifestPath := filepath.Join(manifestDir, "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	outDir := filepath.Join(root, "out")
	if err := MergeRuntimeConfig(MergeRequest{ArtifactPath: manifestPath, OutputDir: outDir}); err != nil {
		t.Fatalf("MergeRuntimeConfig() error = %v", err)
	}

	functions := readYAMLFile(t, filepath.Join(outDir, "functions.yml"))
	fns := asMap(functions["functions"])
	hello := asMap(fns["hello"])
	if hello["handler"] != "b.handler" {
		t.Fatalf("expected hello handler b.handler, got %#v", hello["handler"])
	}
	defaults := asMap(functions["defaults"])
	env := asMap(defaults["environment"])
	if env["LOG_LEVEL"] != "INFO" {
		t.Fatalf("existing default should win: %#v", env["LOG_LEVEL"])
	}
	if env["TRACE"] != "1" {
		t.Fatalf("missing default key should be backfilled: %#v", env["TRACE"])
	}

	routing := readYAMLFile(t, filepath.Join(outDir, "routing.yml"))
	routes := asSlice(routing["routes"])
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	route := asMap(routes[0])
	if route["function"] != "bye" {
		t.Fatalf("expected route to be overwritten by b, got %#v", route["function"])
	}

	imageImport := readJSONFile(t, filepath.Join(outDir, "image-import.json"))
	images := imageImport["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("expected one image import entry, got %d", len(images))
	}
}

func TestApply_ValidatesRequiredSecretEnv(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "a")
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), map[string]any{"functions": map[string]any{}})
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), map[string]any{"routes": []any{}})

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:      "../a",
				RuntimeConfigDir:  "config",
				RequiredSecretEnv: []string{"X_API_KEY", "AUTH_PASS"},
				SourceTemplate:    ArtifactSourceTemplate{Path: "/tmp/a.yaml", SHA256: "sha-a"},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID("/tmp/a.yaml", nil, "sha-a")
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(root, "out")
	err := Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir})
	if err == nil {
		t.Fatal("expected error for missing --secret-env")
	}

	secretEnv := filepath.Join(root, "secrets.env")
	if err := os.WriteFile(secretEnv, []byte("X_API_KEY=abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir, SecretEnvPath: secretEnv})
	if err == nil {
		t.Fatal("expected error for missing AUTH_PASS")
	}

	if err := os.WriteFile(secretEnv, []byte("X_API_KEY=abc\nAUTH_PASS=pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir, SecretEnvPath: secretEnv}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func writeYAMLFile(t *testing.T, path string, value map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readYAMLFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
