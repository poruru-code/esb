// Where: cli/internal/command/branding_test.go
// What: Tests for fixed CLI naming in user-facing output.
// Why: Ensure usage/help always render the canonical command name.
package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsUsesFixedCLIName(t *testing.T) {
	var buf bytes.Buffer
	code := runNoArgs(&buf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	output := buf.String()
	if !strings.Contains(output, "esb deploy --template") {
		t.Fatalf("expected fixed cli name in usage output, got: %q", output)
	}
}
