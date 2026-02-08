// Where: cli/internal/usecase/deploy/runtime_config_test.go
// What: Unit tests for runtime config sync error handling.
// Why: Prevent silent success when container copy fails.
package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncRuntimeConfigToTarget_ContainerCopyFailureIsReturned(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	runner := &runtimeConfigRunner{
		runFunc: func(args ...string) error {
			if len(args) > 0 && args[0] == "cp" {
				return errors.New("container copy failed")
			}
			return nil
		},
	}
	workflow := Workflow{ComposeRunner: runner}

	err := workflow.syncRuntimeConfigToTarget(stagingDir, runtimeConfigTarget{ContainerID: "ctr-1"})
	if err == nil {
		t.Fatal("expected sync error, got nil")
	}
	if !strings.Contains(err.Error(), "copy config to container") {
		t.Fatalf("expected container copy context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "container copy failed") {
		t.Fatalf("expected underlying container error, got: %v", err)
	}
}

func TestSyncRuntimeConfigToTarget_JoinsContainerAndVolumeErrors(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "functions.yml"), []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	runner := &runtimeConfigRunner{
		runFunc: func(args ...string) error {
			if len(args) == 0 {
				return nil
			}
			switch args[0] {
			case "cp":
				return errors.New("container copy failed")
			case "run":
				return errors.New("volume copy failed")
			default:
				return nil
			}
		},
	}
	workflow := Workflow{ComposeRunner: runner}

	err := workflow.syncRuntimeConfigToTarget(
		stagingDir,
		runtimeConfigTarget{
			ContainerID: "ctr-1",
			VolumeName:  "vol-1",
		},
	)
	if err == nil {
		t.Fatal("expected sync error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "container copy failed") {
		t.Fatalf("expected container error in joined message, got: %v", err)
	}
	if !strings.Contains(msg, "volume copy failed") {
		t.Fatalf("expected volume error in joined message, got: %v", err)
	}
}

type runtimeConfigRunner struct {
	runFunc func(args ...string) error
}

func (r *runtimeConfigRunner) Run(_ context.Context, _ string, _ string, args ...string) error {
	if r.runFunc == nil {
		return nil
	}
	return r.runFunc(args...)
}

func (r *runtimeConfigRunner) RunOutput(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (r *runtimeConfigRunner) RunQuiet(_ context.Context, _ string, _ string, args ...string) error {
	if r.runFunc == nil {
		return nil
	}
	return r.runFunc(args...)
}
