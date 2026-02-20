package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestExecuteRuntimeProbeFeedsObservationToApply(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	setRuntimeStackForTest(t, manifestPath, artifactcore.RuntimeStackMeta{
		APIVersion: artifactcore.RuntimeStackAPIVersion,
		Mode:       "docker",
		ESBVersion: "latest",
	})

	result, err := Execute(Input{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runner:       &recordCommandRunner{},
		RuntimeProbe: func(_ artifactcore.ArtifactManifest) (*artifactcore.RuntimeObservation, []string, error) {
			return &artifactcore.RuntimeObservation{
				Mode:       "docker",
				ESBVersion: "latest",
				Source:     "test",
			}, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
}

func TestExecuteRuntimeProbeFailureAddsWarning(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	setRuntimeStackForTest(t, manifestPath, artifactcore.RuntimeStackMeta{
		APIVersion: artifactcore.RuntimeStackAPIVersion,
		Mode:       "docker",
		ESBVersion: "latest",
	})

	result, err := Execute(Input{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runner:       &recordCommandRunner{},
		RuntimeProbe: func(_ artifactcore.ArtifactManifest) (*artifactcore.RuntimeObservation, []string, error) {
			return nil, nil, errors.New("docker unavailable")
		},
	})
	if err != nil {
		t.Fatalf("execute should continue when runtime probe fails: %v", err)
	}
	if !containsMessage(result.Warnings, "runtime compatibility probe failed") {
		t.Fatalf("expected probe failure warning in result: %#v", result.Warnings)
	}
}

func TestExecuteRuntimeProbeWarningAndFailureAreBothReported(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	setRuntimeStackForTest(t, manifestPath, artifactcore.RuntimeStackMeta{
		APIVersion: artifactcore.RuntimeStackAPIVersion,
		Mode:       "docker",
		ESBVersion: "latest",
	})

	result, err := Execute(Input{
		ArtifactPath: manifestPath,
		OutputDir:    filepath.Join(root, "out"),
		Runner:       &recordCommandRunner{},
		RuntimeProbe: func(_ artifactcore.ArtifactManifest) (*artifactcore.RuntimeObservation, []string, error) {
			return nil, []string{"probe warning"}, errors.New("docker unavailable")
		},
	})
	if err != nil {
		t.Fatalf("execute should not fail on runtime probe error: %v", err)
	}
	if !containsMessage(result.Warnings, "probe warning") {
		t.Fatalf("expected probe warning in result: %#v", result.Warnings)
	}
	if !containsMessage(result.Warnings, "runtime compatibility probe failed") {
		t.Fatalf("expected probe failure warning in result: %#v", result.Warnings)
	}
}

func setRuntimeStackForTest(t *testing.T, manifestPath string, runtime artifactcore.RuntimeStackMeta) {
	t.Helper()
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest.RuntimeStack = runtime
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest with runtime_stack: %v", err)
	}
}

func containsMessage(messages []string, pattern string) bool {
	for _, message := range messages {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}
