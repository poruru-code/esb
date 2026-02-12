// Where: services/agent/internal/runtime/image_naming_test.go
// What: Tests for agent image name resolution helpers.
// Why: Ensure sanitization and prefix selection are correct.
package runtime

import (
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestResolveFunctionImageNameUsesMetaPrefix(t *testing.T) {
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

func TestResolveFunctionImageTagUsesEnv(t *testing.T) {
	t.Setenv(meta.EnvPrefix+"_TAG", "v1.2.3")
	if got := ResolveFunctionImageTag(); got != "v1.2.3" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagPrefersCanonicalTag(t *testing.T) {
	t.Setenv("TAG", "v2.0.0")
	t.Setenv(meta.EnvPrefix+"_TAG", "v1.2.3")
	t.Setenv("ESB_TAG", "v1.0.0")
	if got := ResolveFunctionImageTag(); got != "v2.0.0" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagFallsBackToLegacyTag(t *testing.T) {
	t.Setenv("TAG", "")
	t.Setenv(meta.EnvPrefix+"_TAG", "")
	t.Setenv("ESB_TAG", "v1.1.0")
	if got := ResolveFunctionImageTag(); got != "v1.1.0" {
		t.Fatalf("unexpected tag: %s", got)
	}
}

func TestResolveFunctionImageTagDefaultsLatest(t *testing.T) {
	t.Setenv(meta.EnvPrefix+"_TAG", "")
	if got := ResolveFunctionImageTag(); got != "latest" {
		t.Fatalf("unexpected tag: %s", got)
	}
}
