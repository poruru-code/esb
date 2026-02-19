package artifactcore

import "testing"

func TestMissingSecretKeysErrorMessageIsSorted(t *testing.T) {
	err := MissingSecretKeysError{Keys: []string{"AUTH_PASS", "X_API_KEY", "A_KEY"}}
	got := err.Error()
	want := "missing required secret env keys: AUTH_PASS, A_KEY, X_API_KEY"
	if got != want {
		t.Fatalf("error message=%q want=%q", got, want)
	}
}

func TestMissingReferencedPathErrorMessage(t *testing.T) {
	err := MissingReferencedPathError{Path: "/tmp/missing.yml"}
	got := err.Error()
	want := "referenced path not found: /tmp/missing.yml"
	if got != want {
		t.Fatalf("error message=%q want=%q", got, want)
	}
}
