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
