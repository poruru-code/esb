package deployops

import "testing"

func TestResolveRuntimeFunctionRegistryPrefersHostOverRegistryFallback(t *testing.T) {
	t.Setenv("CONTAINER_REGISTRY", "")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5512")
	t.Setenv("REGISTRY", "registry:5010/")

	if got := resolveRuntimeFunctionRegistry(); got != "127.0.0.1:5512" {
		t.Fatalf("resolveRuntimeFunctionRegistry() = %q, want %q", got, "127.0.0.1:5512")
	}
}

func TestNormalizeFunctionImageRefForRuntimeUsesHostWhenContainerRegistryUnset(t *testing.T) {
	t.Setenv("CONTAINER_REGISTRY", "")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5512")
	t.Setenv("REGISTRY", "registry:5010")

	normalized, rewritten := normalizeFunctionImageRefForRuntime("registry:5010/esb-lambda-echo:e2e-test")
	if !rewritten {
		t.Fatal("expected image ref rewrite")
	}
	if normalized != "127.0.0.1:5512/esb-lambda-echo:e2e-test" {
		t.Fatalf("normalizeFunctionImageRefForRuntime() = %q", normalized)
	}
}
