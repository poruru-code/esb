// Where: cli/internal/infra/compose/project_files_test.go
// What: Tests for resolving compose config files from running projects.
// Why: Ensure compose file discovery mirrors docker compose project metadata.
package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestResolveComposeFilesFromProject(t *testing.T) {
	root := t.TempDir()
	fileA := filepath.Join(root, "docker-compose.yml")
	fileB := filepath.Join(root, "docker-compose.docker.yml")
	if err := writeTestFile(fileA, "test"); err != nil {
		t.Fatalf("write compose file A: %v", err)
	}
	if err := writeTestFile(fileB, "test"); err != nil {
		t.Fatalf("write compose file B: %v", err)
	}

	client := &fakeDockerClient{
		containers: []container.Summary{
			{
				State: "running",
				Labels: map[string]string{
					ComposeProjectLabel:     "esb-test",
					ComposeConfigFilesLabel: "docker-compose.yml, docker-compose.docker.yml, missing.yml",
					ComposeWorkingDirLabel:  root,
				},
			},
		},
	}

	result, err := ResolveComposeFilesFromProject(t.Context(), client, "esb-test")
	if err != nil {
		t.Fatalf("resolve compose files: %v", err)
	}
	if result.SetCount != 1 {
		t.Fatalf("expected set count 1, got %d", result.SetCount)
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 existing files, got %d", len(result.Files))
	}
	if len(result.Missing) != 1 {
		t.Fatalf("expected 1 missing file, got %d", len(result.Missing))
	}
	if result.Missing[0] != filepath.Join(root, "missing.yml") {
		t.Fatalf("unexpected missing file: %s", result.Missing[0])
	}
}

func writeTestFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
