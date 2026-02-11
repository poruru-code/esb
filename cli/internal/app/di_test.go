// Where: cli/internal/app/di_test.go
// What: Unit tests for dependency wiring defaults.
// Why: Ensure runtime env application is wired at app layer, not command layer.
package app

import (
	"testing"
)

func TestNewDeployRuntimeDepsWiresRuntimeApply(t *testing.T) {
	deps := newDeployRuntimeDeps(func(string) (string, error) { return "", nil })
	if deps.ApplyRuntimeEnv == nil {
		t.Fatal("expected ApplyRuntimeEnv to be configured")
	}
	if deps.RuntimeEnvResolver == nil {
		t.Fatal("expected RuntimeEnvResolver to be configured")
	}
	if deps.DockerClient == nil {
		t.Fatal("expected DockerClient factory to be configured")
	}
}

func TestBuildDependenciesWiresRuntimeApply(t *testing.T) {
	deps, closer, err := BuildDependencies(nil)
	if err != nil {
		t.Fatalf("build dependencies: %v", err)
	}
	if closer != nil {
		_ = closer.Close()
	}
	if deps.Deploy.Runtime.ApplyRuntimeEnv == nil {
		t.Fatal("expected Deploy.Runtime.ApplyRuntimeEnv to be configured")
	}
}
