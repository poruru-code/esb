package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/meta"
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

func TestResolveRuntimeMetaIncludesDigestsAndVersions(t *testing.T) {
	projectDir := t.TempDir()
	writeTestRuntimeAssets(t, projectDir)

	meta, err := resolveRuntimeMeta(projectDir)
	if err != nil {
		t.Fatalf("resolveRuntimeMeta() error = %v", err)
	}
	if meta.Hooks.APIVersion != runtimeHooksAPIVersion {
		t.Fatalf("hooks api_version = %q, want %q", meta.Hooks.APIVersion, runtimeHooksAPIVersion)
	}
	if meta.Renderer.Name != templateRendererName {
		t.Fatalf("renderer name = %q, want %q", meta.Renderer.Name, templateRendererName)
	}
	if meta.Renderer.APIVersion != templateRendererAPIVersion {
		t.Fatalf("renderer api_version = %q, want %q", meta.Renderer.APIVersion, templateRendererAPIVersion)
	}
	if meta.Hooks.PythonSitecustomizeDigest == "" {
		t.Fatal("python sitecustomize digest must not be empty")
	}
}

func TestResolveRuntimeMetaDoesNotRequireJavaJars(t *testing.T) {
	projectDir := t.TempDir()
	writeTestRuntimeAssets(t, projectDir)

	if err := os.Remove(filepath.Join(projectDir, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")); err != nil {
		t.Fatalf("remove java agent jar: %v", err)
	}
	if err := os.Remove(filepath.Join(projectDir, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")); err != nil {
		t.Fatalf("remove java wrapper jar: %v", err)
	}

	meta, err := resolveRuntimeMeta(projectDir)
	if err != nil {
		t.Fatalf("resolveRuntimeMeta() error = %v", err)
	}
	if meta.Hooks.PythonSitecustomizeDigest == "" {
		t.Fatal("python sitecustomize digest must not be empty")
	}
}
