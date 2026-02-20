package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/meta"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestSanitizePathSegmentBlocksDotSegments(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "default"},
		{name: "dot", value: ".", want: "default"},
		{name: "dot dot", value: "..", want: "default"},
		{name: "slash replaced", value: "demo/dev", want: "demo-dev"},
		{name: "backslash replaced", value: "demo\\dev", want: "demo-dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizePathSegment(tt.value); got != tt.want {
				t.Fatalf("sanitizePathSegment(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestResolveDeployArtifactManifestPathPreventsTraversalByDotSegments(t *testing.T) {
	projectDir := t.TempDir()
	got := resolveDeployArtifactManifestPath(projectDir, "..", "..")
	want := filepath.Join(projectDir, meta.HomeDir, "artifacts", "default", "default", artifactManifestFileName)
	if got != want {
		t.Fatalf("resolveDeployArtifactManifestPath() = %q, want %q", got, want)
	}
}

func TestResolveDeployArtifactManifestPathUsesRelativeOverride(t *testing.T) {
	projectDir := t.TempDir()
	got := resolveDeployArtifactManifestPath(projectDir, "esb", "dev", "e2e/artifacts/dev/artifact.yml")
	want := filepath.Join(projectDir, "e2e", "artifacts", "dev", "artifact.yml")
	if got != want {
		t.Fatalf("resolveDeployArtifactManifestPath() override = %q, want %q", got, want)
	}
}

func TestResolveDeployArtifactManifestPathUsesAbsoluteOverride(t *testing.T) {
	projectDir := t.TempDir()
	abs := filepath.Join(projectDir, "custom", "artifact.yml")
	got := resolveDeployArtifactManifestPath(projectDir, "esb", "dev", abs)
	if got != abs {
		t.Fatalf("resolveDeployArtifactManifestPath() absolute override = %q, want %q", got, abs)
	}
}

func TestNormalizeSourceTemplatePathUsesProjectRelativePath(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "e2e", "fixtures", "template.e2e.yaml")
	got := normalizeSourceTemplatePath(projectDir, templatePath)
	want := filepath.ToSlash(filepath.Join("e2e", "fixtures", "template.e2e.yaml"))
	if got != want {
		t.Fatalf("normalizeSourceTemplatePath() = %q, want %q", got, want)
	}
}

func TestNormalizeSourceTemplatePathKeepsAbsolutePathOutsideProject(t *testing.T) {
	projectDir := t.TempDir()
	external := filepath.Join(t.TempDir(), "template.yaml")
	got := normalizeSourceTemplatePath(projectDir, external)
	if got != filepath.Clean(external) {
		t.Fatalf("normalizeSourceTemplatePath() external = %q, want %q", got, filepath.Clean(external))
	}
}

func TestNormalizeSourceTemplatePathWithoutProjectDirReturnsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	setWorkingDir(t, root)
	rel := filepath.Join("templates", "template.yaml")
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got := normalizeSourceTemplatePath("", rel)
	want, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("normalizeSourceTemplatePath() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveTemplateArtifactRootResolvesRelativeSummaryToAbsolute(t *testing.T) {
	root := t.TempDir()
	setWorkingDir(t, root)
	templateRel := filepath.Join("templates", "sample.yaml")
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, templateRel), []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got, err := resolveTemplateArtifactRoot(templateRel, "", "dev")
	if err != nil {
		t.Fatalf("resolveTemplateArtifactRoot() error = %v", err)
	}
	want, err := filepath.Abs(filepath.Join("templates", meta.OutputDir, "dev"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("resolveTemplateArtifactRoot() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveTemplateArtifactRootHonorsAbsoluteOutputDir(t *testing.T) {
	root := t.TempDir()
	templateAbs := filepath.Join(root, "templates", "sample.yaml")
	if err := os.MkdirAll(filepath.Dir(templateAbs), 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(templateAbs, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	outputAbs := filepath.Join(root, "out")

	got, err := resolveTemplateArtifactRoot(templateAbs, outputAbs, "dev")
	if err != nil {
		t.Fatalf("resolveTemplateArtifactRoot() error = %v", err)
	}
	want := filepath.Join(outputAbs, "dev")
	if got != filepath.Clean(want) {
		t.Fatalf("resolveTemplateArtifactRoot() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestCloneStringValues(t *testing.T) {
	t.Run("empty map returns nil", func(t *testing.T) {
		if got := cloneStringValues(nil); got != nil {
			t.Fatalf("cloneStringValues(nil) = %#v, want nil", got)
		}
		if got := cloneStringValues(map[string]string{}); got != nil {
			t.Fatalf("cloneStringValues(empty) = %#v, want nil", got)
		}
	})

	t.Run("returns independent copy", func(t *testing.T) {
		in := map[string]string{
			"ParamA": "value-a",
		}
		out := cloneStringValues(in)
		if out["ParamA"] != "value-a" {
			t.Fatalf("cloned value mismatch: %#v", out)
		}
		in["ParamA"] = "changed"
		if out["ParamA"] != "value-a" {
			t.Fatalf("clone must not track source mutation: %#v", out)
		}
	})
}

func TestManifestGenerationDoesNotRequireJavaJars(t *testing.T) {
	projectDir := t.TempDir()
	writeTestRuntimeAssets(t, projectDir)

	if err := os.Remove(filepath.Join(projectDir, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")); err != nil {
		t.Fatalf("remove java agent jar: %v", err)
	}
	if err := os.Remove(filepath.Join(projectDir, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")); err != nil {
		t.Fatalf("remove java wrapper jar: %v", err)
	}

	inputs := deployInputs{
		ProjectDir: projectDir,
		Project:    "demo",
		Env:        "dev",
		Mode:       "docker",
		Templates: []deployTemplateInput{
			{
				TemplatePath: filepath.Join(projectDir, "template.yaml"),
				OutputDir:    filepath.Join(projectDir, ".esb", "dev"),
				Parameters: map[string]string{
					"Stage": "dev",
				},
			},
		},
	}
	if err := os.WriteFile(inputs.Templates[0].TemplatePath, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	manifestPath, err := writeDeployArtifactManifest(inputs, false, "", "latest")
	if err != nil {
		t.Fatalf("writeDeployArtifactManifest() error = %v", err)
	}
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadArtifactManifest() error = %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("manifest artifacts len = %d, want 1", len(manifest.Artifacts))
	}
	if manifest.Artifacts[0].ID == "" {
		t.Fatalf("artifact id should be generated")
	}
	if manifest.RuntimeStack.APIVersion != artifactcore.RuntimeStackAPIVersion {
		t.Fatalf("runtime stack api_version = %q, want %q", manifest.RuntimeStack.APIVersion, artifactcore.RuntimeStackAPIVersion)
	}
	if manifest.RuntimeStack.Mode != "docker" {
		t.Fatalf("runtime stack mode = %q, want docker", manifest.RuntimeStack.Mode)
	}
	if manifest.RuntimeStack.ESBVersion != "latest" {
		t.Fatalf("runtime stack esb_version = %q, want latest", manifest.RuntimeStack.ESBVersion)
	}
}
