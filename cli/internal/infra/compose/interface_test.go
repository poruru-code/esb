// Where: cli/internal/infra/compose/interface_test.go
// What: Tests for command execution runner output routing.
// Why: Ensure ExecRunner honors injected writers for deterministic CLI output capture.
package compose

import (
	"bytes"
	"context"
	"testing"
)

func TestExecRunnerRunUsesInjectedWriters(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	runner := ExecRunner{
		Out:    &out,
		ErrOut: &errOut,
	}
	if err := runner.Run(context.Background(), "", "sh", "-c", "printf out; printf err >&2"); err != nil {
		t.Fatalf("run command: %v", err)
	}
	if out.String() != "out" {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
	if errOut.String() != "err" {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}
