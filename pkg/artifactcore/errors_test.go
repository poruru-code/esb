package artifactcore

import "testing"

func TestMissingReferencedPathErrorMessage(t *testing.T) {
	err := MissingReferencedPathError{Path: "/tmp/missing.yml"}
	got := err.Error()
	want := "referenced path not found: /tmp/missing.yml"
	if got != want {
		t.Fatalf("error message=%q want=%q", got, want)
	}
}
