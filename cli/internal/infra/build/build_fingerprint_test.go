// Where: cli/internal/infra/build/build_fingerprint_test.go
// What: Tests for image build fingerprint calculation.
// Why: Ensure mutable image-source bases trigger rebuild when upstream digest changes.
package build

import (
	"path/filepath"
	"testing"

	"github.com/poruru-code/esb/cli/internal/domain/template"
)

func TestBuildImageFingerprintChangesWhenImageSourceDigestChanges(t *testing.T) {
	outputDir := t.TempDir()
	writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}\n")
	writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []\n")
	writeTestFile(t, filepath.Join(outputDir, "functions", "lambda-image", "Dockerfile"), "FROM scratch\n")

	functions := []template.FunctionSpec{
		{
			Name:        "lambda-image",
			ImageSource: "public.ecr.aws/example/repo:latest",
		},
	}

	first, err := buildImageFingerprint(
		outputDir,
		"esb-test",
		"dev",
		"sha256:base",
		functions,
		map[string]string{
			"public.ecr.aws/example/repo:latest": "public.ecr.aws/example/repo@sha256:111",
		},
	)
	if err != nil {
		t.Fatalf("first fingerprint: %v", err)
	}
	second, err := buildImageFingerprint(
		outputDir,
		"esb-test",
		"dev",
		"sha256:base",
		functions,
		map[string]string{
			"public.ecr.aws/example/repo:latest": "public.ecr.aws/example/repo@sha256:222",
		},
	)
	if err != nil {
		t.Fatalf("second fingerprint: %v", err)
	}
	if first == second {
		t.Fatalf("expected fingerprint change when image-source digest changes, got %q", first)
	}
}
