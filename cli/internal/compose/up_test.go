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

func (f *fakeRunner) RunQuiet(_ context.Context, dir, name string, args ...string) error {
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
	rootDir := createTempComposeFiles(t)

	files, err := ResolveComposeFiles(rootDir, ModeDocker, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		filepath.Join(rootDir, "docker-compose.docker.yml"),
	}

	if !equalStringSlices(files, expected) {
		t.Errorf("unexpected files:\n\tgot:  %v\n\twant: %v", files, expected)
	}
}

func TestResolveComposeFilesContainerdMode(t *testing.T) {
	rootDir := createTempComposeFiles(t)

	files, err := ResolveComposeFiles(rootDir, ModeContainerd, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		filepath.Join(rootDir, "docker-compose.containerd.yml"),
	}

	if !equalStringSlices(files, expected) {
		t.Errorf("unexpected files:\n\tgot:  %v\n\twant: %v", files, expected)
	}
}

func TestResolveComposeFilesFirecrackerMode(t *testing.T) {
	rootDir := createTempComposeFiles(t)

	files, err := ResolveComposeFiles(rootDir, ModeFirecracker, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		filepath.Join(rootDir, "docker-compose.fc.yml"),
	}

	if !equalStringSlices(files, expected) {
		t.Errorf("unexpected files:\n\tgot:  %v\n\twant: %v", files, expected)
	}
}

func TestResolveComposeFilesFirecrackerNode(t *testing.T) {
	rootDir := createTempComposeFiles(t)

	files, err := ResolveComposeFiles(rootDir, ModeFirecracker, "node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		filepath.Join(rootDir, "docker-compose.fc-node.yml"),
	}

	if !equalStringSlices(files, expected) {
		t.Errorf("unexpected files:\n\tgot:  %v\n\twant: %v", files, expected)
	}
}

func TestResolveComposeFilesInvalidTarget(t *testing.T) {
	root := t.TempDir()
	required := []string{
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
	rootDir := createTempComposeFiles(t)
	runner := &fakeRunner{}
	ctx := context.Background()

	opts := UpOptions{
		RootDir: rootDir,
		Mode:    ModeDocker,
		Project: "esb-default",
		Detach:  true,
	}

	err := UpProject(ctx, runner, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedArgs := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(rootDir, "docker-compose.docker.yml"),
		"up", "-d",
	}

	if !reflect.DeepEqual(runner.args, expectedArgs) {
		t.Errorf("unexpected args: %v", runner.args)
	}
}

func TestUpProjectPassesEnvFile(t *testing.T) {
	rootDir := createTempComposeFiles(t)
	runner := &fakeRunner{}
	ctx := context.Background()

	opts := UpOptions{
		RootDir: rootDir,
		Mode:    ModeDocker,
		Project: "esb-test",
		EnvFile: "/path/to/.env.docker",
		Detach:  true,
	}

	err := UpProject(ctx, runner, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedArgs := []string{
		"compose",
		"-p", "esb-test",
		"-f", filepath.Join(rootDir, "docker-compose.docker.yml"),
		"--env-file", "/path/to/.env.docker",
		"up", "-d",
	}

	if !reflect.DeepEqual(runner.args, expectedArgs) {
		t.Errorf("unexpected args:\n\tgot:  %v\n\twant: %v", runner.args, expectedArgs)
	}
}

// Helper to create all necessary files for tests
func createTempComposeFiles(t *testing.T) string {
	dir := t.TempDir()
	files := []string{
		"docker-compose.docker.yml",
		"docker-compose.docker.yml",
		"docker-compose.containerd.yml",
		"docker-compose.fc.yml",
		"docker-compose.fc-node.yml",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// equalStringSlices checks if two string slices are equal
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
