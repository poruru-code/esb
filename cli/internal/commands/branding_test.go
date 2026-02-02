// Where: cli/internal/commands/branding_test.go
// What: Tests for brand-aware CLI output.
// Why: Ensure usage/completion reflect the configured CLI name.
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsUsesBrandName(t *testing.T) {
	t.Setenv("CLI_CMD", "acme")

	var buf bytes.Buffer
	code := runNoArgs(&buf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	output := buf.String()
	if !strings.Contains(output, "acme deploy --template") {
		t.Fatalf("expected brand name in usage output, got: %q", output)
	}
	if strings.Contains(output, "esb deploy --template") {
		t.Fatalf("unexpected default brand in output: %q", output)
	}
}

func TestCompletionBashUsesBrandName(t *testing.T) {
	t.Setenv("CLI_CMD", "acme")

	var buf bytes.Buffer
	code := runCompletionBash(CLI{}, &buf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	output := buf.String()
	if !strings.Contains(output, "_acme_completion") {
		t.Fatalf("expected brand completion function, got: %q", output)
	}
	if !strings.Contains(output, "complete -F _acme_completion acme") {
		t.Fatalf("expected brand completion registration, got: %q", output)
	}
}
