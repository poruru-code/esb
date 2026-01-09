// Where: cli/internal/compose/stop_test.go
// What: Tests for compose stop/logs helpers.
// Why: Ensure stop/logs command construction matches expectations.
package compose

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStopProjectBuildsCommand(t *testing.T) {
	root := t.TempDir()
	writeStopComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := StopOptions{
		RootDir: root,
		Project: "esb-default",
		Mode:    ModeDocker,
	}

	if err := StopProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"stop",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func TestLogsProjectBuildsCommand(t *testing.T) {
	root := t.TempDir()
	writeStopComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := LogsOptions{
		RootDir:    root,
		Project:    "esb-default",
		Mode:       ModeDocker,
		Follow:     true,
		Tail:       50,
		Timestamps: true,
		Service:    "gateway",
	}

	if err := LogsProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"logs",
		"--follow",
		"--tail", "50",
		"--timestamps",
		"gateway",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func writeStopComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}
}
