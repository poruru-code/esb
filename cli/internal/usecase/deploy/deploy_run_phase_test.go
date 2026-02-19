package deploy

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestRunApplyPhaseReturnsRegistryWaitError(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")
	repoRoot := newTestRepoRoot(t)

	req := Request{
		Context: state.Context{
			ProjectDir:     repoRoot,
			ComposeProject: "esb-dev",
			TemplatePath:   filepath.Join(repoRoot, "template.yaml"),
			Env:            "dev",
			Mode:           "docker",
		},
	}

	workflow := Workflow{
		RegistryWaiter: func(string, time.Duration) error {
			return errors.New("registry timeout")
		},
	}

	err := workflow.runApplyPhase(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "registry not ready") {
		t.Fatalf("expected registry wait context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "registry timeout") {
		t.Fatalf("expected underlying registry error, got: %v", err)
	}
}

func TestRunApplyPhaseReturnsArtifactPathRequiredWhenWaitPasses(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")
	repoRoot := newTestRepoRoot(t)

	req := Request{
		Context: state.Context{
			ProjectDir:     repoRoot,
			ComposeProject: "esb-dev",
			TemplatePath:   filepath.Join(repoRoot, "template.yaml"),
			Env:            "dev",
			Mode:           "docker",
		},
	}

	workflow := Workflow{
		RegistryWaiter: noopRegistryWaiter,
	}

	err := workflow.runApplyPhase(req)
	if !errors.Is(err, artifactcore.ErrArtifactPathRequired) {
		t.Fatalf("expected ErrArtifactPathRequired, got %v", err)
	}
}
