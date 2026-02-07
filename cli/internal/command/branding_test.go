// Where: cli/internal/command/branding_test.go
// What: Tests for brand-aware CLI output.
// Why: Ensure usage output reflects the configured CLI name.
package command

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
