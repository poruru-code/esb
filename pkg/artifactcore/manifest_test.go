package artifactcore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRoundTripAndValidate(t *testing.T) {
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
			},
		},
	}
	if err := WriteArtifactManifest(path, manifest); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}
	if _, err := ReadArtifactManifest(path); err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
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
			},
		},
	}

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
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`future_top_level: true`,
		`artifacts:`,
		`  - artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    future_entry_field: abc`,
		`    source_template:`,
		`      path: /tmp/template-a.yaml`,
		`      sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`,
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

func TestReadArtifactManifestUncheckedSkipsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`artifacts:`,
		`  - artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    source_template:`,
		`      sha256: not-hex`,
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
}

func TestReadArtifactManifestRejectsExplicitEmptySourceTemplatePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`artifacts:`,
		`  - artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    source_template:`,
		`      path: ""`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := ReadArtifactManifest(path)
	if err == nil {
		t.Fatal("expected source_template.path validation error")
	}
	if !strings.Contains(err.Error(), "source_template.path must not be blank") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadArtifactManifestRejectsExplicitEmptySourceTemplateSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb-dev`,
		`env: dev`,
		`mode: docker`,
		`artifacts:`,
		`  - artifact_root: ../service-a/.esb/template-a/dev`,
		`    runtime_config_dir: config`,
		`    source_template:`,
		`      sha256: ""`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := ReadArtifactManifest(path)
	if err == nil {
		t.Fatal("expected source_template.sha256 validation error")
	}
	if !strings.Contains(err.Error(), "source_template.sha256 must not be blank") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestValidateAllowsMissingSourceTemplate(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].SourceTemplate = nil

	if err := manifest.Validate(); err != nil {
		t.Fatalf("expected missing source_template to be allowed, got %v", err)
	}
}

func TestManifestValidateRejectsBlankSourceTemplatePath(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].SourceTemplate = &ArtifactSourceTemplate{Path: "   "}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected source_template.path validation error")
	}
	if !strings.Contains(err.Error(), "source_template.path must not be blank") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestValidateRejectsInvalidSourceTemplateSHA256(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].SourceTemplate = &ArtifactSourceTemplate{SHA256: "abc"}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected source_template.sha256 validation error")
	}
	if !strings.Contains(err.Error(), "source_template.sha256 must be 64 lowercase hex characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestValidateAllowsSourceTemplateWithoutPath(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].SourceTemplate = &ArtifactSourceTemplate{
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("expected source_template without path to be allowed, got %v", err)
	}
}

func TestManifestRoundTripAllowsSourceTemplateWithoutPath(t *testing.T) {
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
				SourceTemplate: &ArtifactSourceTemplate{
					SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
			},
		},
	}

	if err := WriteArtifactManifest(path, manifest); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}
	readBack, err := ReadArtifactManifest(path)
	if err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
	}
	if readBack.Artifacts[0].SourceTemplate == nil {
		t.Fatal("source_template should remain set")
	}
	if readBack.Artifacts[0].SourceTemplate.Path != "" {
		t.Fatalf("source_template.path = %q, want empty", readBack.Artifacts[0].SourceTemplate.Path)
	}
}

func TestManifestValidateRejectsEmptySourceTemplateParameterKey(t *testing.T) {
	manifest := validTestManifest()
	manifest.Artifacts[0].SourceTemplate = &ArtifactSourceTemplate{
		Parameters: map[string]string{"": "x"},
	}

	err := manifest.Validate()
	if err == nil {
		t.Fatal("expected source_template.parameters validation error")
	}
	if !strings.Contains(err.Error(), "source_template.parameters contains empty key") {
		t.Fatalf("unexpected error: %v", err)
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
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
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

func TestManifestValidateRuntimeStackRequiresModeWhenConfigured(t *testing.T) {
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
	return ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: &ArtifactSourceTemplate{
					Path:       "/tmp/template-a.yaml",
					SHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					Parameters: map[string]string{"Stage": "dev"},
				},
			},
		},
	}
}
