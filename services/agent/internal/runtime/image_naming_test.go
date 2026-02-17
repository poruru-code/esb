// Where: services/agent/internal/runtime/image_naming_test.go
// What: Tests for agent image name resolution helpers.
// Why: Ensure sanitization and prefix selection are correct.
package runtime

import (
	"testing"
)

func TestResolveFunctionImageNameUsesDerivedPrefix(t *testing.T) {
	t.Setenv("PROJECT_NAME", "esb-dev")
	t.Setenv("ENV", "dev")
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

func TestResolveFunctionImageNameFailsWhenIdentityMissing(t *testing.T) {
	t.Setenv("ESB_BRAND_SLUG", "")
	t.Setenv("PROJECT_NAME", "")
	t.Setenv("ENV", "")
	t.Setenv("CONTAINERS_NETWORK", "")
	if _, err := ResolveFunctionImageName("lambda"); err == nil {
		t.Fatalf("expected error for missing identity")
	}
}

func TestResolveFunctionImageTagUsesEnv(t *testing.T) {
	t.Setenv("PROJECT_NAME", "esb-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ESB_TAG", "v1.2.3")
	got, err := ResolveFunctionImageTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagUsesDerivedPrefix(t *testing.T) {
	t.Setenv("PROJECT_NAME", "acme-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ACME_TAG", "v2.0.0")
	got, err := ResolveFunctionImageTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.0.0" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagFallsBackToESBTag(t *testing.T) {
	t.Setenv("PROJECT_NAME", "acme-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ACME_TAG", "")
	t.Setenv("ESB_TAG", "legacy")
	got, err := ResolveFunctionImageTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "legacy" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagDefaultsLatest(t *testing.T) {
	t.Setenv("PROJECT_NAME", "esb-dev")
	t.Setenv("ENV", "dev")
	t.Setenv("ESB_TAG", "")
	got, err := ResolveFunctionImageTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "latest" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagFailsWhenIdentityMissing(t *testing.T) {
	t.Setenv("ESB_BRAND_SLUG", "")
	t.Setenv("PROJECT_NAME", "")
	t.Setenv("ENV", "")
	t.Setenv("CONTAINERS_NETWORK", "")
	if _, err := ResolveFunctionImageTag(); err == nil {
		t.Fatalf("expected error for missing identity")
	}
}
