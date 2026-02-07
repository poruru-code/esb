// Where: cli/internal/infra/build/image_import_test.go
// What: Tests for image source normalization and import manifest generation.
// Why: Keep image ref mapping deterministic across deploys.
package build

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
)

func TestBuildImageImportEntry(t *testing.T) {
	entry, needsImport, err := buildImageImportEntry(
		"lambda-image",
		"123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo/name:v1",
		"registry:5010",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !needsImport {
		t.Fatalf("expected import to be required")
	}
	expected := "registry:5010/123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo/name:v1"
	if entry.ImageRef != expected {
		t.Fatalf("unexpected image_ref: %s", entry.ImageRef)
	}
}

func TestBuildImageImportEntryNormalizesHostPort(t *testing.T) {
	entry, needsImport, err := buildImageImportEntry(
		"lambda-image",
		"ghcr.io:443/example/repo:latest",
		"registry:5010",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !needsImport {
		t.Fatalf("expected import to be required")
	}
	expected := "registry:5010/ghcr.io_443/example/repo:latest"
	if entry.ImageRef != expected {
		t.Fatalf("unexpected image_ref: %s", entry.ImageRef)
	}
}

func TestBuildImageImportEntryDigestOnlyTagging(t *testing.T) {
	entry, _, err := buildImageImportEntry(
		"lambda-image",
		"public.ecr.aws/example/repo@sha256:1234567890abcdef",
		"registry:5010",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := "registry:5010/public.ecr.aws/example/repo:sha256-1234567890ab"
	if entry.ImageRef != expected {
		t.Fatalf("unexpected image_ref: %s", entry.ImageRef)
	}
}

func TestResolveImageImportsSkipsInternalSource(t *testing.T) {
	functions := []template.FunctionSpec{
		{
			Name:        "lambda-internal",
			ImageSource: "registry:5010/demo/repo:latest",
		},
	}
	entries, err := resolveImageImports(functions, "registry:5010/")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no import entries, got %d", len(entries))
	}
	if functions[0].ImageRef != "registry:5010/demo/repo:latest" {
		t.Fatalf("unexpected image ref: %s", functions[0].ImageRef)
	}
}
