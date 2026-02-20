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

func TestReadArtifactManifestUncheckedAllowsIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`artifacts:`,
		`  - id: template-deadbeef`,
		`    artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    source_template:`,
		`      path: /tmp/template-a.yaml`,
		`      sha256: sha-a`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := ReadArtifactManifestUnchecked(path)
	if err != nil {
		t.Fatalf("ReadArtifactManifestUnchecked() error = %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected single artifact entry, got %d", len(manifest.Artifacts))
	}
	if manifest.Artifacts[0].ID != "template-deadbeef" {
		t.Fatalf("unexpected id: %q", manifest.Artifacts[0].ID)
	}
}

func TestSyncArtifactIDsRewritesMismatchedEntries(t *testing.T) {
	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ID:               "template-deadbeef",
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:       "/tmp/template-a.yaml",
					SHA256:     "sha-a",
					Parameters: map[string]string{"Stage": "dev"},
				},
			},
			{
				ID:               "template-feedface",
				ArtifactRoot:     "../service-a/.esb/template-b/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template-b.yaml",
					SHA256: "sha-b",
				},
			},
		},
	}
	wantFirst := ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)
	wantSecond := ComputeArtifactID(
		manifest.Artifacts[1].SourceTemplate.Path,
		manifest.Artifacts[1].SourceTemplate.Parameters,
		manifest.Artifacts[1].SourceTemplate.SHA256,
	)

	changed := SyncArtifactIDs(&manifest)
	if changed != 2 {
		t.Fatalf("changed=%d want=2", changed)
	}
	if manifest.Artifacts[0].ID != wantFirst {
		t.Fatalf("first id=%q want=%q", manifest.Artifacts[0].ID, wantFirst)
	}
	if manifest.Artifacts[1].ID != wantSecond {
		t.Fatalf("second id=%q want=%q", manifest.Artifacts[1].ID, wantSecond)
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

func TestWriteArtifactManifestSyncsArtifactIDs(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].ID = "template-deadbeef"
	path := filepath.Join(t.TempDir(), "artifact.yml")

	if err := WriteArtifactManifest(path, manifest); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}
	readBack, err := ReadArtifactManifest(path)
	if err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
	}
	wantID := ComputeArtifactID(
		readBack.Artifacts[0].SourceTemplate.Path,
		readBack.Artifacts[0].SourceTemplate.Parameters,
		readBack.Artifacts[0].SourceTemplate.SHA256,
	)
	if readBack.Artifacts[0].ID != wantID {
		t.Fatalf("artifact id=%q want=%q", readBack.Artifacts[0].ID, wantID)
	}
}

func TestManifestValidateRuntimeStackRequiresModeAndVersionWhenConfigured(t *testing.T) {
	manifest := validTestManifest()
	manifest.RuntimeStack = RuntimeStackMeta{
		APIVersion: RuntimeStackAPIVersion,
	}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected runtime_stack validation error")
	}
	if !strings.Contains(err.Error(), "runtime_stack.mode is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestValidateRuntimeStackRejectsInvalidMode(t *testing.T) {
	manifest := validTestManifest()
	manifest.RuntimeStack = RuntimeStackMeta{
		APIVersion: RuntimeStackAPIVersion,
		Mode:       "firecracker",
		ESBVersion: "latest",
	}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected runtime_stack mode validation error")
	}
	if !strings.Contains(err.Error(), "runtime_stack.mode must be docker or containerd") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestValidateRuntimeStackRejectsInvalidAPIVersion(t *testing.T) {
	manifest := validTestManifest()
	manifest.RuntimeStack = RuntimeStackMeta{
		APIVersion: "abc",
		Mode:       "docker",
		ESBVersion: "latest",
	}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected runtime_stack api_version validation error")
	}
	if !strings.Contains(err.Error(), "runtime_stack.api_version is invalid") {
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
