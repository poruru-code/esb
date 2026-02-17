package command

import (
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

func TestResolveRuntimeMetaIncludesDigestsAndVersions(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve project dir: %v", err)
	}

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
	if meta.Hooks.JavaAgentDigest == "" {
		t.Fatal("java agent digest must not be empty")
	}
	if meta.Hooks.JavaWrapperDigest == "" {
		t.Fatal("java wrapper digest must not be empty")
	}
	if meta.Renderer.TemplateDigest == "" {
		t.Fatal("template digest must not be empty")
	}
}
