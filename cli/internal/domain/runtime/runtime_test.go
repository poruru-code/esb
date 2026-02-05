// Where: cli/internal/domain/runtime/runtime_test.go
// What: Tests for runtime profile resolution.
// Why: Ensure runtime defaults and mappings stay stable.
package runtime

import "testing"

func TestResolveDefaultPython(t *testing.T) {
	profile, err := Resolve("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if profile.Kind != KindPython {
		t.Fatalf("expected python kind, got %s", profile.Kind)
	}
	if profile.Name != "python3.12" {
		t.Fatalf("expected default runtime python3.12, got %s", profile.Name)
	}
	if !profile.UsesSitecustomize {
		t.Fatalf("expected sitecustomize to be enabled for python")
	}
	if !profile.UsesPip {
		t.Fatalf("expected pip to be enabled for python")
	}
	if !profile.NestPythonLayers {
		t.Fatalf("expected python layers to be nested")
	}
	if profile.PythonVersion != "3.12" {
		t.Fatalf("expected python version 3.12, got %s", profile.PythonVersion)
	}
}

func TestResolveJava21(t *testing.T) {
	profile, err := Resolve("java21")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if profile.Kind != KindJava {
		t.Fatalf("expected java kind, got %s", profile.Kind)
	}
	if !profile.UsesJavaWrapper {
		t.Fatalf("expected java wrapper to be enabled")
	}
	if profile.JavaBaseImage != "public.ecr.aws/lambda/java:21" {
		t.Fatalf("expected java base image, got %s", profile.JavaBaseImage)
	}
}

func TestResolveUnknownRuntime(t *testing.T) {
	if _, err := Resolve("nodejs18.x"); err == nil {
		t.Fatalf("expected error for unknown runtime")
	}
}

func TestCodeUriTargetDir(t *testing.T) {
	profile, err := Resolve("java21")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dir := profile.CodeUriTargetDir("/tmp/app.jar"); dir != "lib" {
		t.Fatalf("expected lib target dir, got %s", dir)
	}
	if dir := profile.CodeUriTargetDir("/tmp/app.zip"); dir != "" {
		t.Fatalf("expected empty target dir for non-jar, got %s", dir)
	}
}
