package config

import (
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func TestNormalizeOutputDir(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty defaults", in: "", want: meta.OutputDir},
		{name: "trim spaces", in: "  out  ", want: "out"},
		{name: "trim trailing slash", in: "dist/", want: "dist"},
		{name: "trim trailing backslash", in: "dist\\", want: "dist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeOutputDir(tt.in)
			if got != tt.want {
				t.Fatalf("NormalizeOutputDir(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveOutputSummary(t *testing.T) {
	templatePath := filepath.Join("/repo", "services", "demo", "template.yaml")

	t.Run("default output dir", func(t *testing.T) {
		got := ResolveOutputSummary(templatePath, "", "dev")
		want := filepath.Join("/repo", "services", "demo", meta.OutputDir, "dev")
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("ResolveOutputSummary() = %q, want %q", got, want)
		}
	})

	t.Run("relative output dir", func(t *testing.T) {
		got := ResolveOutputSummary(templatePath, "build", "dev")
		want := filepath.Join("/repo", "services", "demo", "build", "dev")
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("ResolveOutputSummary() = %q, want %q", got, want)
		}
	})

	t.Run("absolute output dir", func(t *testing.T) {
		got := ResolveOutputSummary(templatePath, "/tmp/esb-out", "dev")
		want := filepath.Join("/tmp/esb-out", "dev")
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("ResolveOutputSummary() = %q, want %q", got, want)
		}
	})
}
