package command

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

var (
	errTestError      = errors.New("test error")
	errSomeOtherError = errors.New("some other error")
)

func TestExitWithError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	code := exitWithError(&buf, errTestError)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	want := "✗ test error\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestExitWithSuggestion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	code := exitWithSuggestion(&buf, "Something went wrong.", []string{"try this", "or that"})

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	output := buf.String()
	if !contains(output, "⚠️  Something went wrong.") {
		t.Errorf("missing error message in output: %s", output)
	}
	if !contains(output, "Next steps:") {
		t.Errorf("missing 'Next steps:' in output: %s", output)
	}
	if !contains(output, "try this") {
		t.Errorf("missing suggestion in output: %s", output)
	}
}

func TestExitWithSuggestionAndAvailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	code := exitWithSuggestionAndAvailable(&buf,
		"Environment not found.",
		[]string{"esb env use <name>"},
		[]string{"dev", "prod"},
	)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	output := buf.String()
	if !contains(output, "⚠️  Environment not found.") {
		t.Errorf("missing error message: %s", output)
	}
	if !contains(output, "Next steps:") {
		t.Errorf("missing 'Next steps:': %s", output)
	}
	if !contains(output, "Available:") {
		t.Errorf("missing 'Available:': %s", output)
	}
	if !contains(output, "dev") || !contains(output, "prod") {
		t.Errorf("missing available items: %s", output)
	}
}

func TestHandleParseError_GenericError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	code := handleParseError([]string{"build"}, errSomeOtherError, Dependencies{}, &buf)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	output := buf.String()
	if !contains(output, "✗ some other error") {
		t.Errorf("expected error to be printed: %s", output)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
