// Where: cli/internal/compose/up_test.go
// What: Tests for compose up helpers.
// Why: Ensure command construction and file resolution are stable.
package compose

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeRunner struct {
	dir  string
	name string
	args []string
	err  error
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) error {
	f.dir = dir
	f.name = name
	f.args = append([]string{}, args...)
	return f.err
}

func (f *fakeRunner) RunOutput(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.dir = dir
	f.name = name
	f.args = append([]string{}, args...)
	return nil, f.err
}

func TestResolveComposeFilesDockerMode(t *testing.T) {
	root := t.TempDir()
	required := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	}
	for _, name := range required {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	files, err := ResolveComposeFiles(root, ModeDocker, "control")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := []string{
		filepath.Join(root, "docker-compose.yml"),
		filepath.Join(root, "docker-compose.worker.yml"),
		filepath.Join(root, "docker-compose.docker.yml"),
	}
	if !reflect.DeepEqual(files, expected) {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestResolveComposeFilesContainerdMode(t *testing.T) {
	root := t.TempDir()
	required := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.containerd.yml",
	}
	for _, name := range required {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	files, err := ResolveComposeFiles(root, ModeContainerd, "control")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := []string{
		filepath.Join(root, "docker-compose.yml"),
		filepath.Join(root, "docker-compose.worker.yml"),
		filepath.Join(root, "docker-compose.registry.yml"),
		filepath.Join(root, "docker-compose.containerd.yml"),
	}
	if !reflect.DeepEqual(files, expected) {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestResolveComposeFilesFirecrackerMode(t *testing.T) {
	root := t.TempDir()
	required := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.fc.yml",
	}
	for _, name := range required {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	files, err := ResolveComposeFiles(root, ModeFirecracker, "control")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := []string{
		filepath.Join(root, "docker-compose.yml"),
		filepath.Join(root, "docker-compose.worker.yml"),
		filepath.Join(root, "docker-compose.registry.yml"),
		filepath.Join(root, "docker-compose.fc.yml"),
	}
	if !reflect.DeepEqual(files, expected) {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestResolveComposeFilesInvalidTarget(t *testing.T) {
	root := t.TempDir()
	required := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	}
	for _, name := range required {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	_, err := ResolveComposeFiles(root, ModeDocker, "compute")
	if err == nil {
		t.Fatalf("expected error for unsupported target")
	}
}

func TestUpProjectBuildsCommand(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	}
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	runner := &fakeRunner{}
	opts := UpOptions{
		RootDir: root,
		Project: "esb-default",
		Detach:  true,
	}

	if err := UpProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if runner.name != "docker" {
		t.Fatalf("expected docker command, got %s", runner.name)
	}
	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"up",
		"-d",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
	if runner.dir != root {
		t.Fatalf("unexpected working dir: %s", runner.dir)
	}
}

func TestUpProjectPassesEnvFile(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.docker.yml",
	}
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}

	runner := &fakeRunner{}
	opts := UpOptions{
		RootDir: root,
		Project: "esb-test",
		Detach:  true,
		EnvFile: "/path/to/.env.docker",
	}

	if err := UpProject(context.Background(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-test",
		"-f", filepath.Join(root, "docker-compose.yml"),
		"-f", filepath.Join(root, "docker-compose.worker.yml"),
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"--env-file", "/path/to/.env.docker",
		"up",
		"-d",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args:\ngot:  %v\nwant: %v", runner.args, expected)
	}
}
