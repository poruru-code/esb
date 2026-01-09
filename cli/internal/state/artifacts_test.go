// Where: cli/internal/state/artifacts_test.go
// What: Tests for build artifact detection.
// Why: Ensure state detection can reliably identify built outputs.
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasBuildArtifacts(t *testing.T) {
	t.Run("missing config and dockerfile", func(t *testing.T) {
		outputDir := filepath.Join(t.TempDir(), "output", "default")
		ok, err := HasBuildArtifacts(outputDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ok {
			t.Fatalf("expected false when artifacts are missing")
		}
	})

	t.Run("config present but dockerfile missing", func(t *testing.T) {
		outputDir := filepath.Join(t.TempDir(), "output", "default")
		configDir := filepath.Join(outputDir, "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir config: %v", err)
		}
		if err := writeFile(filepath.Join(configDir, "functions.yml")); err != nil {
			t.Fatalf("write functions.yml: %v", err)
		}
		if err := writeFile(filepath.Join(configDir, "routing.yml")); err != nil {
			t.Fatalf("write routing.yml: %v", err)
		}

		ok, err := HasBuildArtifacts(outputDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ok {
			t.Fatalf("expected false when Dockerfile is missing")
		}
	})

	t.Run("dockerfile present but routing missing", func(t *testing.T) {
		outputDir := filepath.Join(t.TempDir(), "output", "default")
		configDir := filepath.Join(outputDir, "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir config: %v", err)
		}
		if err := writeFile(filepath.Join(configDir, "functions.yml")); err != nil {
			t.Fatalf("write functions.yml: %v", err)
		}

		dockerfileDir := filepath.Join(outputDir, "functions", "hello")
		if err := os.MkdirAll(dockerfileDir, 0o755); err != nil {
			t.Fatalf("mkdir dockerfile dir: %v", err)
		}
		if err := writeFile(filepath.Join(dockerfileDir, "Dockerfile")); err != nil {
			t.Fatalf("write Dockerfile: %v", err)
		}

		ok, err := HasBuildArtifacts(outputDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ok {
			t.Fatalf("expected false when routing.yml is missing")
		}
	})

	t.Run("config and dockerfile present", func(t *testing.T) {
		outputDir := filepath.Join(t.TempDir(), "output", "default")
		configDir := filepath.Join(outputDir, "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir config: %v", err)
		}
		if err := writeFile(filepath.Join(configDir, "functions.yml")); err != nil {
			t.Fatalf("write functions.yml: %v", err)
		}
		if err := writeFile(filepath.Join(configDir, "routing.yml")); err != nil {
			t.Fatalf("write routing.yml: %v", err)
		}

		dockerfileDir := filepath.Join(outputDir, "functions", "hello")
		if err := os.MkdirAll(dockerfileDir, 0o755); err != nil {
			t.Fatalf("mkdir dockerfile dir: %v", err)
		}
		if err := writeFile(filepath.Join(dockerfileDir, "Dockerfile")); err != nil {
			t.Fatalf("write Dockerfile: %v", err)
		}

		ok, err := HasBuildArtifacts(outputDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ok {
			t.Fatalf("expected true when artifacts are present")
		}
	})
}

func writeFile(path string) error {
	return os.WriteFile(path, []byte("test"), 0o644)
}
