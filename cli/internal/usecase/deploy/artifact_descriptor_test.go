package deploy

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArtifactDescriptorValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name       string
		descriptor ArtifactDescriptor
		wantErr    string
	}{
		{
			name:       "missing schema version",
			descriptor: ArtifactDescriptor{},
			wantErr:    "schema_version",
		},
		{
			name: "missing runtime config dir",
			descriptor: ArtifactDescriptor{
				SchemaVersion: ArtifactSchemaVersionV1,
				Project:       "esb-dev",
				Env:           "dev",
				Mode:          "docker",
			},
			wantErr: "runtime_config_dir",
		},
		{
			name: "valid descriptor",
			descriptor: ArtifactDescriptor{
				SchemaVersion:    ArtifactSchemaVersionV1,
				Project:          "esb-dev",
				Env:              "dev",
				Mode:             "docker",
				RuntimeConfigDir: "runtime-config",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.descriptor.Validate()
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

func TestArtifactDescriptorRejectsAbsoluteAndEscapingPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantErr  string
		expected bool
	}{
		{
			name:    "absolute path",
			path:    "/tmp/runtime-config",
			wantErr: "relative path",
		},
		{
			name:    "escaping path",
			path:    "../runtime-config",
			wantErr: "escape artifact root",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			descriptor := ArtifactDescriptor{
				SchemaVersion:    ArtifactSchemaVersionV1,
				Project:          "esb-dev",
				Env:              "dev",
				Mode:             "docker",
				RuntimeConfigDir: tc.path,
			}
			err := descriptor.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}

func TestArtifactDescriptorReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")

	want := ArtifactDescriptor{
		SchemaVersion:    ArtifactSchemaVersionV1,
		Project:          "esb-dev",
		Env:              "dev",
		Mode:             "docker",
		RuntimeConfigDir: "runtime-config",
		BundleManifest:   "bundle/manifest.json",
		ImagePrewarm:     "all",
		RequiredSecretEnv: []string{
			"X_API_KEY",
			"AUTH_PASS",
			"X_API_KEY",
		},
		Templates: []ArtifactTemplate{
			{Path: "templates/b.yaml", SHA256: "bb"},
			{Path: "templates/a.yaml", SHA256: "aa"},
		},
		RuntimeMeta: ArtifactRuntimeMeta{
			Hooks: RuntimeHooksMeta{
				APIVersion:      "1.0",
				JavaAgentDigest: "digest-java-agent",
			},
			Renderer: RendererMeta{
				Name:       "esb-cli-embedded-templates",
				APIVersion: "1.0",
			},
		},
	}

	if err := WriteArtifactDescriptor(path, want); err != nil {
		t.Fatalf("WriteArtifactDescriptor() error = %v", err)
	}

	got, err := ReadArtifactDescriptor(path)
	if err != nil {
		t.Fatalf("ReadArtifactDescriptor() error = %v", err)
	}

	expected := want
	expected.RequiredSecretEnv = []string{"AUTH_PASS", "X_API_KEY"}
	expected.Templates = []ArtifactTemplate{
		{Path: "templates/a.yaml", SHA256: "aa"},
		{Path: "templates/b.yaml", SHA256: "bb"},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("round-trip mismatch\n got: %#v\nwant: %#v", got, expected)
	}
}

func TestArtifactDescriptorResolvePaths(t *testing.T) {
	descriptorPath := filepath.Join(t.TempDir(), "artifacts", "artifact.json")
	descriptor := ArtifactDescriptor{
		SchemaVersion:    ArtifactSchemaVersionV1,
		Project:          "esb-dev",
		Env:              "dev",
		Mode:             "docker",
		RuntimeConfigDir: "runtime-config",
		BundleManifest:   "bundle/manifest.json",
	}

	runtimeConfigDir, err := descriptor.ResolveRuntimeConfigDir(descriptorPath)
	if err != nil {
		t.Fatalf("ResolveRuntimeConfigDir() error = %v", err)
	}
	wantRuntimeConfigDir := filepath.Join(filepath.Dir(descriptorPath), "runtime-config")
	if runtimeConfigDir != wantRuntimeConfigDir {
		t.Fatalf("ResolveRuntimeConfigDir() = %q, want %q", runtimeConfigDir, wantRuntimeConfigDir)
	}

	bundleManifest, err := descriptor.ResolveBundleManifest(descriptorPath)
	if err != nil {
		t.Fatalf("ResolveBundleManifest() error = %v", err)
	}
	wantBundleManifest := filepath.Join(filepath.Dir(descriptorPath), "bundle", "manifest.json")
	if bundleManifest != wantBundleManifest {
		t.Fatalf("ResolveBundleManifest() = %q, want %q", bundleManifest, wantBundleManifest)
	}
}
