package workflows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestDeployWorkflowRunSuccess(t *testing.T) {
	builder := &recordBuilder{}
	envApplier := &recordEnvApplier{}
	ui := &testUI{}
	runner := &fakeComposeRunner{}

	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_SKIP_GATEWAY_ALIGN", "1")

	// Use the actual repo root for testing
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Go up to the repo root (we're in cli/internal/workflows)
	repoRoot = filepath.Join(repoRoot, "..", "..", "..")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// Set ESB_REPO so ResolveRepoRoot can find it
	t.Setenv("ESB_REPO", repoRoot)

	ctx := state.Context{
		ProjectDir:     repoRoot,
		ComposeProject: "esb-dev",
	}
	req := DeployRequest{
		Context:      ctx,
		Env:          "dev",
		Mode:         "docker",
		TemplatePath: filepath.Join(repoRoot, "template.yaml"),
		OutputDir:    ".out",
		Parameters:   map[string]string{"ParamA": "value"},
		Tag:          "v1.2.3",
		NoCache:      true,
	}

	workflow := NewDeployWorkflow(builder, envApplier, ui, runner)
	// Use a mock registry checker to avoid waiting for real registry
	workflow.RegistryChecker = &fakeRegistryChecker{}
	// Note: This test uses the actual repo root. It will fail if the repo
	// structure doesn't match expectations (e.g., missing compose files).
	if err := workflow.Run(req); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.applied) != 1 {
		t.Fatalf("expected env applier to be called once, got %d", len(envApplier.applied))
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called once, got %d", len(builder.requests))
	}

	got := builder.requests[0]
	if got.ProjectDir != req.Context.ProjectDir {
		t.Fatalf("project dir mismatch: %s", got.ProjectDir)
	}
	if got.ProjectName != req.Context.ComposeProject {
		t.Fatalf("project name mismatch: %s", got.ProjectName)
	}
	if got.TemplatePath != req.TemplatePath {
		t.Fatalf("template path mismatch: %s", got.TemplatePath)
	}
	if got.Env != req.Env {
		t.Fatalf("env mismatch: %s", got.Env)
	}
	if got.Mode != req.Mode {
		t.Fatalf("mode mismatch: %s", got.Mode)
	}
	if got.OutputDir != req.OutputDir {
		t.Fatalf("output dir mismatch: %s", got.OutputDir)
	}
	if got.Tag != req.Tag {
		t.Fatalf("tag mismatch: %s", got.Tag)
	}
	if got.Parameters["ParamA"] != "value" {
		t.Fatalf("parameters mismatch")
	}
	if !got.NoCache {
		t.Fatalf("expected no-cache to be true")
	}

	if len(ui.success) != 1 || !strings.Contains(ui.success[0], "Deploy complete") {
		t.Fatalf("expected deploy success message")
	}
}

func TestDeployWorkflowRunMissingBuilder(t *testing.T) {
	workflow := NewDeployWorkflow(nil, nil, nil, nil)
	err := workflow.Run(DeployRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "builder port is not configured") {
		t.Fatalf("expected builder missing error, got %v", err)
	}
}
