// Where: cli/internal/command/deploy_inputs_compose_test.go
// What: Tests for compose-file normalization helpers.
// Why: Keep compose file handling deterministic across repeated inputs.
package command

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeComposeFiles(t *testing.T) {
	baseDir := t.TempDir()
	absFile := filepath.Join(baseDir, "docker-compose.override.yml")
	got := normalizeComposeFiles([]string{
		" docker-compose.yml ",
		"",
		"./docker-compose.yml",
		absFile,
		absFile,
	}, baseDir)

	want := []string{
		filepath.Join(baseDir, "docker-compose.yml"),
		absFile,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected compose files: got=%v want=%v", got, want)
	}
}

func TestNormalizeComposeFilesEmptyInput(t *testing.T) {
	if got := normalizeComposeFiles(nil, ""); got != nil {
		t.Fatalf("expected nil for empty files, got %v", got)
	}
}
