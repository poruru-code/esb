package composeprovision

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordRunner struct {
	runCalls      int
	runQuietCalls int
	lastArgs      []string
}

func (r *recordRunner) Run(_ context.Context, _, _ string, args ...string) error {
	r.runCalls++
	r.lastArgs = append([]string(nil), args...)
	return nil
}

func (r *recordRunner) RunQuiet(_ context.Context, _, _ string, args ...string) error {
	r.runQuietCalls++
	r.lastArgs = append([]string(nil), args...)
	return nil
}

type failRunner struct{}

func (failRunner) Run(_ context.Context, _, _ string, _ ...string) error {
	return errors.New("boom")
}

func (failRunner) RunQuiet(_ context.Context, _, _ string, _ ...string) error {
	return errors.New("boom")
}

func TestBuildArgs(t *testing.T) {
	got := buildArgs(Request{
		ComposeProject: "esb-dev",
		ComposeFiles:   []string{"a.yml", " ", "b.yml"},
		EnvFile:        ".env",
		NoDeps:         true,
		NoWarnOrphans:  true,
	})
	want := []string{
		"compose",
		"-f",
		"a.yml",
		"-f",
		"b.yml",
		"--no-warn-orphans",
		"-p",
		"esb-dev",
		"--env-file",
		".env",
		"--profile",
		"deploy",
		"run",
		"--rm",
		"--no-deps",
		"provisioner",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildArgs() = %#v, want %#v", got, want)
	}
}

func TestExecuteUsesRunWhenVerbose(t *testing.T) {
	runner := &recordRunner{}
	err := Execute(context.Background(), runner, "/tmp", Request{Verbose: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.runCalls != 1 || runner.runQuietCalls != 0 {
		t.Fatalf("unexpected run counts: run=%d quiet=%d", runner.runCalls, runner.runQuietCalls)
	}
}

func TestExecuteUsesRunQuietByDefault(t *testing.T) {
	runner := &recordRunner{}
	err := Execute(context.Background(), runner, "/tmp", Request{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.runCalls != 0 || runner.runQuietCalls != 1 {
		t.Fatalf("unexpected run counts: run=%d quiet=%d", runner.runCalls, runner.runQuietCalls)
	}
}

func TestExecuteWrapsRunnerError(t *testing.T) {
	err := Execute(context.Background(), failRunner{}, "/tmp", Request{})
	if err == nil || !strings.Contains(err.Error(), "run provisioner") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}
