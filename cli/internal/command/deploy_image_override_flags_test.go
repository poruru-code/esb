package command

import (
	"reflect"
	"testing"
)

func TestParseFunctionOverrideFlag(t *testing.T) {
	got, err := parseFunctionOverrideFlag(
		[]string{"lambda-a=java21", "lambda-b=public.ecr.aws/example/repo:latest"},
		"--image-runtime",
	)
	if err != nil {
		t.Fatalf("parse function override flag: %v", err)
	}
	want := map[string]string{
		"lambda-a": "java21",
		"lambda-b": "public.ecr.aws/example/repo:latest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected parsed map: got=%v want=%v", got, want)
	}
}

func TestParseFunctionOverrideFlagRejectsInvalidInput(t *testing.T) {
	cases := []string{"lambda-a", "=java21", "lambda-a="}
	for _, value := range cases {
		if _, err := parseFunctionOverrideFlag([]string{value}, "--image-runtime"); err == nil {
			t.Fatalf("expected parse error for %q", value)
		}
	}
}

func TestFilterFunctionOverrides(t *testing.T) {
	overrides := map[string]string{
		"lambda-a": "java21",
		"lambda-b": "python",
	}
	got := filterFunctionOverrides(overrides, []string{"lambda-b", "lambda-c"})
	want := map[string]string{
		"lambda-b": "python",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered overrides: got=%v want=%v", got, want)
	}
}
