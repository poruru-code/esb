// Where: cli/internal/infra/compose/client_test.go
// What: Tests for Docker client construction.
// Why: Ensure we can construct a Docker client without side effects.
package compose

import "testing"

func TestNewDockerClient(t *testing.T) {
	client, err := NewDockerClient()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatalf("expected client")
	}
	if closer, ok := client.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
