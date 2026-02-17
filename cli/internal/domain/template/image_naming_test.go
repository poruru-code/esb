// Where: cli/internal/domain/template/image_naming_test.go
// What: Tests for image name sanitization and collision detection.
// Why: Ensure function image naming rules remain deterministic and safe.
package template

import "testing"

func TestImageSafeName(t *testing.T) {
	name, err := imageSafeName("My Func!!")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if name != "my-func" {
		t.Fatalf("unexpected image name: %s", name)
	}

	name, err = imageSafeName("___")
	if err == nil {
		t.Fatalf("expected error for empty image name, got %s", name)
	}
}

func TestApplyImageNamesDetectsCollisions(t *testing.T) {
	functions := []FunctionSpec{
		{Name: "Foo"},
		{Name: "foo"},
	}
	if err := ApplyImageNames(functions); err == nil {
		t.Fatalf("expected collision error, got nil")
	}
}

func TestApplyImageNamesIncludesImageSourceFunctions(t *testing.T) {
	functions := []FunctionSpec{
		{Name: "FromSource", ImageSource: "public.ecr.aws/example/repo:latest"},
	}
	if err := ApplyImageNames(functions); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if functions[0].ImageName != "fromsource" {
		t.Fatalf("expected image name for image-source function, got %q", functions[0].ImageName)
	}
}
