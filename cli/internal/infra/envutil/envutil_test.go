// Where: cli/internal/infra/envutil/envutil_test.go
// What: Tests for canonical/legacy environment compatibility helpers.
// Why: Keep migration behavior stable while phasing out ESB_* keys.
package envutil

import (
	"os"
	"testing"
)

func TestGetCompatEnvPrefersCanonical(t *testing.T) {
	t.Setenv("ENV_PREFIX", "APP")
	t.Setenv("TAG", "v3")
	t.Setenv("APP_TAG", "v2")
	t.Setenv("ESB_TAG", "v1")

	value, source := GetCompatEnv("TAG", "TAG")
	if value != "v3" || source != "TAG" {
		t.Fatalf("got value=%q source=%q", value, source)
	}
}

func TestGetCompatEnvFallsBackToPrefixed(t *testing.T) {
	t.Setenv("ENV_PREFIX", "APP")
	t.Setenv("APP_TAG", "v2")

	value, source := GetCompatEnv("TAG", "TAG")
	if value != "v2" || source != "APP_TAG" {
		t.Fatalf("got value=%q source=%q", value, source)
	}
}

func TestGetCompatEnvFallsBackToLegacy(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")
	t.Setenv("ESB_TAG", "v1")

	value, source := GetCompatEnv("TAG", "TAG")
	if value != "v1" || source != "ESB_TAG" {
		t.Fatalf("got value=%q source=%q", value, source)
	}
}

func TestSetCompatEnvWritesCanonicalAndPrefixed(t *testing.T) {
	t.Setenv("ENV_PREFIX", "APP")
	t.Setenv("ESB_TAG", "legacy")

	if err := SetCompatEnv("TAG", "TAG", "v9"); err != nil {
		t.Fatalf("SetCompatEnv: %v", err)
	}
	if got := os.Getenv("TAG"); got != "v9" {
		t.Fatalf("TAG=%q", got)
	}
	if got := os.Getenv("APP_TAG"); got != "v9" {
		t.Fatalf("APP_TAG=%q", got)
	}
	if got := os.Getenv("ESB_TAG"); got != "legacy" {
		t.Fatalf("ESB_TAG must not be overwritten, got %q", got)
	}
}

func TestSetCompatEnvCanonicalOnlyWithoutPrefix(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")

	if err := SetCompatEnv("TAG", "TAG", "v9"); err != nil {
		t.Fatalf("SetCompatEnv: %v", err)
	}
	if got := os.Getenv("TAG"); got != "v9" {
		t.Fatalf("TAG=%q", got)
	}
}

func TestSetCompatEnvFailsWithoutTargets(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")
	if err := SetCompatEnv("MODE", "", "docker"); err != errEnvPrefixRequired {
		t.Fatalf("unexpected error: %v", err)
	}
}
