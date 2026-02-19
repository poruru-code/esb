package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestExecuteValidatesBeforePrepare(t *testing.T) {
	runner := &recordCommandRunner{}
	_, err := Execute(Input{
		ArtifactPath: "artifact.yml",
		OutputDir:    "",
		Runner:       runner,
	})
	if !errors.Is(err, artifactcore.ErrOutputDirRequired) {
		t.Fatalf("expected ErrOutputDirRequired, got %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner must not be called when validation fails: %#v", runner.commands)
	}
}

func TestExecuteRunsPrepareAndApply(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	outputDir := filepath.Join(root, "out")
	runner := &recordCommandRunner{}

	result, err := Execute(Input{
		ArtifactPath: manifestPath,
		OutputDir:    outputDir,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "functions.yml")); err != nil {
		t.Fatalf("functions.yml not merged: %v", err)
	}
	if len(runner.commands) == 0 {
		t.Fatal("expected prepare image commands")
	}
}

func TestNormalizeInputTrimsValues(t *testing.T) {
	normalized, err := normalizeInput(Input{
		ArtifactPath:  "  artifact.yml  ",
		OutputDir:     "  out  ",
		SecretEnvPath: "  secret.env  ",
	})
	if err != nil {
		t.Fatalf("normalizeInput error = %v", err)
	}
	if normalized.ArtifactPath != "artifact.yml" || normalized.OutputDir != "out" || normalized.SecretEnvPath != "secret.env" {
		t.Fatalf("unexpected normalized input: %#v", normalized)
	}
}
