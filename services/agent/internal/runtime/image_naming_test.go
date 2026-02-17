// Where: services/agent/internal/runtime/image_naming_test.go
// What: Tests for agent image name resolution helpers.
// Why: Ensure sanitization and prefix selection are correct.
package runtime

import (
	"testing"
)

func TestResolveFunctionImageNameUsesDefaultPrefix(t *testing.T) {
	name, err := ResolveFunctionImageName("Lambda_One")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := "esb-lambda_one"
	if name != expected {
		t.Fatalf("unexpected image name: %s", name)
	}
}

func TestResolveFunctionImageNameRejectsEmpty(t *testing.T) {
	if _, err := ResolveFunctionImageName("___"); err == nil {
		t.Fatalf("expected error for empty image name")
	}
}

func TestResolveFunctionImageTagUsesEnv(t *testing.T) {
	t.Setenv("ESB_TAG", "v1.2.3")
	if got := ResolveFunctionImageTag(); got != "v1.2.3" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagUsesDerivedPrefix(t *testing.T) {
	t.Setenv("PROJECT_NAME", "acme-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ACME_TAG", "v2.0.0")
	if got := ResolveFunctionImageTag(); got != "v2.0.0" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagFallsBackToESBTag(t *testing.T) {
	t.Setenv("PROJECT_NAME", "acme-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ACME_TAG", "")
	t.Setenv("ESB_TAG", "legacy")
	if got := ResolveFunctionImageTag(); got != "legacy" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagDefaultsLatest(t *testing.T) {
	t.Setenv("ESB_TAG", "")
	if got := ResolveFunctionImageTag(); got != "latest" {
		t.Fatalf("unexpected tag: %s", got)
	}
}
