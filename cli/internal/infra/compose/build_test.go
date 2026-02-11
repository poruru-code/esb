// Where: cli/internal/infra/compose/build_test.go
// What: Tests for compose build env.
// Why: Ensure build commands are wired correctly.
package compose

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildProjectBuildsCommand(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeDocker,
		Services: []string{"gateway", "agent"},
	}

	if err := BuildProject(t.Context(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := []string{
		"compose",
		"-p", "esb-default",
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
		"docker-compose.containerd.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeContainerd,
		Services: []string{"gateway", "agent"},
	}

	if err := BuildProject(t.Context(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
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

func TestBuildProjectUsesNoCacheFlag(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
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

	if err := BuildProject(t.Context(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"compose",
		"-p", "esb-default",
		"-f", filepath.Join(root, "docker-compose.docker.yml"),
		"build",
		"--no-cache",
		"gateway",
	}
	if !reflect.DeepEqual(runner.args, expected) {
		t.Fatalf("unexpected args: %v", runner.args)
	}
}

func TestBuildProjectStreamsWhenEnabled(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root,
		"docker-compose.docker.yml",
	)

	runner := &fakeRunner{}
	opts := BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeDocker,
		Services: []string{"gateway"},
		Stream:   true,
	}

	if err := BuildProject(t.Context(), runner, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if runner.lastCall != "run" {
		t.Fatalf("expected stream build to use Run, got %s", runner.lastCall)
	}
}

func TestBuildProjectWritesFailureOutputToConfiguredErrOut(t *testing.T) {
	root := t.TempDir()
	writeComposeFiles(t, root, "docker-compose.docker.yml")

	runner := &fakeRunner{
		output:    []byte("compose failed output\n"),
		outputErr: errors.New("boom"),
	}
	var errOut bytes.Buffer

	err := BuildProject(t.Context(), runner, BuildOptions{
		RootDir:  root,
		Project:  "esb-default",
		Mode:     ModeDocker,
		Services: []string{"gateway"},
		ErrOut:   &errOut,
	})
	if err == nil {
		t.Fatal("expected build error")
	}
	if errOut.String() != "compose failed output\n" {
		t.Fatalf("unexpected err output: %q", errOut.String())
	}
}

func writeComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
	}
}
