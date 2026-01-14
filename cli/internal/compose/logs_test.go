// Where: cli/internal/compose/logs_test.go
// What: Tests for compose logs helpers including GetContainerEnv.
// Why: Ensure container env retrieval works correctly.
package compose

import (
	"context"
	"testing"
)

type fakeLogsRunner struct {
	output []byte
	err    error
}

func (f *fakeLogsRunner) Run(_ context.Context, _, _ string, _ ...string) error {
	return f.err
}

func (f *fakeLogsRunner) RunQuiet(_ context.Context, _, _ string, _ ...string) error {
	return f.err
}

func (f *fakeLogsRunner) RunOutput(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
}

func TestGetContainerEnvParsesOutput(t *testing.T) {
	runner := &fakeLogsRunner{
		output: []byte("FOO=bar\nBAZ=qux\nEMPTY=\n"),
	}

	envVars, err := GetContainerEnv(context.Background(), runner, "test-container")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{"FOO=bar", "BAZ=qux", "EMPTY="}
	if len(envVars) != len(expected) {
		t.Fatalf("expected %d vars, got %d: %v", len(expected), len(envVars), envVars)
	}

	for i, exp := range expected {
		if envVars[i] != exp {
			t.Fatalf("expected envVars[%d]=%q, got %q", i, exp, envVars[i])
		}
	}
}

func TestGetContainerEnvNilRunner(t *testing.T) {
	_, err := GetContainerEnv(context.Background(), nil, "test")
	if err == nil {
		t.Fatalf("expected error for nil runner")
	}
}
