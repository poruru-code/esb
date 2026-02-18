package deploy

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArtifactManifestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		manifest ArtifactManifest
		wantErr  string
	}{
		{
			name:     "missing schema version",
			manifest: ArtifactManifest{},
			wantErr:  "schema_version",
		},
		{
			name: "missing artifacts",
			manifest: ArtifactManifest{
				SchemaVersion: ArtifactSchemaVersionV1,
				Project:       "esb-dev",
				Env:           "dev",
				Mode:          "docker",
			},
			wantErr: "artifacts",
		},
		{
			name: "valid manifest",
			manifest: ArtifactManifest{
				SchemaVersion: ArtifactSchemaVersionV1,
				Project:       "esb-dev",
				Env:           "dev",
				Mode:          "docker",
				Artifacts: []ArtifactEntry{
					{
						ID:               ComputeArtifactID("/tmp/template-a.yaml", map[string]string{"Stage": "dev"}, "sha-a"),
						ArtifactRoot:     "../service-a/.esb/template-a/dev",
						RuntimeConfigDir: "config",
						SourceTemplate: ArtifactSourceTemplate{
							Path:       "/tmp/template-a.yaml",
							SHA256:     "sha-a",
							Parameters: map[string]string{"Stage": "dev"},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.manifest.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}

func TestArtifactManifestRejectsInvalidEntryPaths(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		bundle  string
		wantErr string
	}{
		{
			name:    "absolute runtime config path",
			runtime: "/tmp/runtime-config",
			wantErr: "runtime_config_dir",
		},
		{
			name:    "escaping runtime config path",
			runtime: "../runtime-config",
			wantErr: "escape artifact root",
		},
		{
			name:    "escaping bundle path",
			runtime: "config",
			bundle:  "../../bundle/manifest.json",
			wantErr: "bundle_manifest",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := ArtifactManifest{
				SchemaVersion: ArtifactSchemaVersionV1,
				Project:       "esb-dev",
				Env:           "dev",
				Mode:          "docker",
				Artifacts: []ArtifactEntry{
					{
						ID:               ComputeArtifactID("/tmp/template-a.yaml", nil, ""),
						ArtifactRoot:     "../service-a/.esb/template-a/dev",
						RuntimeConfigDir: tc.runtime,
						BundleManifest:   tc.bundle,
						SourceTemplate: ArtifactSourceTemplate{
							Path: "/tmp/template-a.yaml",
						},
					},
				},
			}
			err := manifest.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}

func TestArtifactManifestReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yml")

	want := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:      "../service-a/.esb/template-a/dev",
				RuntimeConfigDir:  "config",
				BundleManifest:    "bundle/manifest.json",
				RequiredSecretEnv: []string{"X_API_KEY", "AUTH_PASS", "X_API_KEY"},
				SourceTemplate: ArtifactSourceTemplate{
					Path:       "/tmp/template-a.yaml",
					SHA256:     "sha-a",
					Parameters: map[string]string{"Stage": "dev"},
				},
				RuntimeMeta: ArtifactRuntimeMeta{
					Renderer: RendererMeta{
						Name:       "esb-cli-embedded-templates",
						APIVersion: "1.0",
					},
				},
			},
			{
				ArtifactRoot:     "../service-b/.esb/template-b/dev",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template-b.yaml",
					SHA256: "sha-b",
				},
			},
		},
	}
	want.Artifacts[0].ID = ComputeArtifactID(
		want.Artifacts[0].SourceTemplate.Path,
		want.Artifacts[0].SourceTemplate.Parameters,
		want.Artifacts[0].SourceTemplate.SHA256,
	)
	want.Artifacts[1].ID = ComputeArtifactID(
		want.Artifacts[1].SourceTemplate.Path,
		want.Artifacts[1].SourceTemplate.Parameters,
		want.Artifacts[1].SourceTemplate.SHA256,
	)

	if err := WriteArtifactManifest(path, want); err != nil {
		t.Fatalf("WriteArtifactManifest() error = %v", err)
	}

	got, err := ReadArtifactManifest(path)
	if err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
	}

	expected := want
	expected.Artifacts = append([]ArtifactEntry(nil), want.Artifacts...)
	expected.Artifacts[0].RequiredSecretEnv = []string{"AUTH_PASS", "X_API_KEY"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("round-trip mismatch\n got: %#v\nwant: %#v", got, expected)
	}
}

func TestArtifactManifestResolvePaths(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "artifacts", "artifact.yml")
	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ID:               ComputeArtifactID("/tmp/template-a.yaml", nil, "sha-a"),
				ArtifactRoot:     "../service-a/.esb/template-a/dev",
				RuntimeConfigDir: "config",
				BundleManifest:   "bundle/manifest.json",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "/tmp/template-a.yaml",
					SHA256: "sha-a",
				},
			},
		},
	}

	artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, 0)
	if err != nil {
		t.Fatalf("ResolveArtifactRoot() error = %v", err)
	}
	wantRoot := filepath.Join(filepath.Dir(manifestPath), "..", "service-a", ".esb", "template-a", "dev")
	if artifactRoot != filepath.Clean(wantRoot) {
		t.Fatalf("ResolveArtifactRoot() = %q, want %q", artifactRoot, filepath.Clean(wantRoot))
	}

	runtimeConfigDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, 0)
	if err != nil {
		t.Fatalf("ResolveRuntimeConfigDir() error = %v", err)
	}
	wantRuntimeConfigDir := filepath.Join(filepath.Clean(wantRoot), "config")
	if runtimeConfigDir != wantRuntimeConfigDir {
		t.Fatalf("ResolveRuntimeConfigDir() = %q, want %q", runtimeConfigDir, wantRuntimeConfigDir)
	}

	bundleManifest, err := manifest.ResolveBundleManifest(manifestPath, 0)
	if err != nil {
		t.Fatalf("ResolveBundleManifest() error = %v", err)
	}
	wantBundleManifest := filepath.Join(filepath.Clean(wantRoot), "bundle", "manifest.json")
	if bundleManifest != wantBundleManifest {
		t.Fatalf("ResolveBundleManifest() = %q, want %q", bundleManifest, wantBundleManifest)
	}
}

func TestComputeArtifactIDDeterministic(t *testing.T) {
	first := ComputeArtifactID(
		"./svc/../template.yaml",
		map[string]string{"Stage": "dev", "Region": "ap-northeast-1"},
		"abcd",
	)
	second := ComputeArtifactID(
		"template.yaml",
		map[string]string{"Region": "ap-northeast-1", "Stage": "dev"},
		"abcd",
	)
	if first != second {
		t.Fatalf("id mismatch: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "template-") {
		t.Fatalf("unexpected id prefix: %q", first)
	}
}
