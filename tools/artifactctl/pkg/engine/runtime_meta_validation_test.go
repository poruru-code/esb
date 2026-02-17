package engine

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
			JavaAgentDigest: "deadbeef",
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
	if !strings.Contains(warnings.String(), "java_agent_digest mismatch") {
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
	if !strings.Contains(err.Error(), "java_agent_digest mismatch") {
		t.Fatalf("unexpected strict error: %v", err)
	}
}

func TestApply_RuntimeDigestVerificationWithoutRepoRoot(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeArtifactFixtureManifest(t, root, ArtifactRuntimeMeta{
		Renderer: RendererMeta{
			TemplateDigest: "cafebabe",
		},
	})

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
		t.Fatalf("non-strict apply should pass when repo root is unavailable: %v", err)
	}
	if !strings.Contains(warnings.String(), "requires repository root") {
		t.Fatalf("expected repository root warning, got %q", warnings.String())
	}

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out-strict"),
		Strict:       true,
	})
	if err == nil {
		t.Fatal("strict apply should fail when digest cannot be verified")
	}
	if !strings.Contains(err.Error(), "requires repository root") {
		t.Fatalf("unexpected strict error: %v", err)
	}
}

func TestApply_RuntimeDigestVerificationAllowsMissingJavaHooksWhenUndeclared(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")

	pythonPath := filepath.Join(
		repoRoot,
		"runtime-hooks",
		"python",
		"sitecustomize",
		"site-packages",
		"sitecustomize.py",
	)
	if err := os.MkdirAll(filepath.Dir(pythonPath), 0o755); err != nil {
		t.Fatalf("mkdir python hook dir: %v", err)
	}
	if err := os.WriteFile(pythonPath, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write python hook: %v", err)
	}

	templatePath := filepath.Join(
		repoRoot,
		"cli",
		"assets",
		"runtime-templates",
		"python",
		"templates",
		"dockerfile.tmpl",
	)
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(templatePath, []byte("FROM python:3.12\n"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	pythonDigest, err := fileSHA256(pythonPath)
	if err != nil {
		t.Fatalf("hash python hook: %v", err)
	}
	templateDigest, err := directoryDigest(filepath.Join(repoRoot, "cli", "assets", "runtime-templates"))
	if err != nil {
		t.Fatalf("hash template dir: %v", err)
	}

	manifestPath := writeArtifactFixtureManifest(t, repoRoot, ArtifactRuntimeMeta{
		Hooks: RuntimeHooksMeta{
			PythonSitecustomizeDigest: pythonDigest,
		},
		Renderer: RendererMeta{
			TemplateDigest: templateDigest,
		},
	})

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	err = Apply(ApplyRequest{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(repoRoot, "out-strict"),
		Strict:       true,
	})
	if err != nil {
		t.Fatalf("strict apply should pass when java digests are undeclared: %v", err)
	}
}

func writeArtifactFixtureManifest(t *testing.T, root string, meta ArtifactRuntimeMeta) string {
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
