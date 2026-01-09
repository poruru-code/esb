// Where: cli/internal/compose/build_test.go
// What: Tests for compose build helpers.
// Why: Ensure build commands are wired correctly.
package compose

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildProjectBuildsCommand(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeDocker,
		Services: []string{"gateway", "agent"},
	}

	if err := BuildProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"build",
		"gateway",
		"agent",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
	if runner.dir != root {
		t.Fatalf("unexpected working dir: %s", runner.dir)
	}
}

func TestBuildProjectAddsRuntimeNodeForContainerd(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.containerd.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeContainerd,
		Services: []string{"gateway", "agent"},
	}

	if err := BuildProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.registry.yml"),
		"-f", filepath.Join(root, "docker-compose.containerd.yml"),
		"build",
		"gateway",
		"agent",
		"runtime-node",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func TestBuildProjectAddsRuntimeNodeForFirecracker(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.fc.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeFirecracker,
		Services: []string{"gateway", "agent"},
	}

	if err := BuildProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.registry.yml"),
		"-f", filepath.Join(root, "docker-compose.fc.yml"),
		"build",
		"gateway",
		"agent",
		"runtime-node",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func TestBuildProjectUsesNoCacheFlag(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeDocker,
		Services: []string{"gateway"},
		NoCache:  true,
	}

	if err := BuildProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"build",
		"--no-cache",
		"gateway",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func writeComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}
}
