// Where: services/agent/internal/runtime/image_naming_test.go
// What: Tests for agent image name resolution helpers.
// Why: Ensure sanitization and prefix selection are correct.
package runtime

import (
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestResolveFunctionImageNameUsesEnvPrefix(t *testing.T) {
	t.Setenv("IMAGE_PREFIX", "acme")
	name, err := ResolveFunctionImageName("My Func")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if name != "acme-my-func" {
		t.Fatalf("unexpected image name: %s", name)
	}
}

func TestResolveFunctionImageNameFallsBackToMeta(t *testing.T) {
	t.Setenv("IMAGE_PREFIX", "")
	name, err := ResolveFunctionImageName("Lambda_One")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := meta.ImagePrefix + "-lambda_one"
	if name != expected {
		t.Fatalf("unexpected image name: %s", name)
	}
}

func TestResolveFunctionImageNameRejectsEmpty(t *testing.T) {
	if _, err := ResolveFunctionImageName("___"); err == nil {
		t.Fatalf("expected error for empty image name")
	}
}
