package deployops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/poruru-code/esb/pkg/artifactcore"
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

func TestExecuteNormalizesOutputFunctionImagesToRuntimeRegistry(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5512")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5512")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return true }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

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

	buildFound := false
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[0] == "docker" && cmd[1] == "buildx" && cmd[2] == "build" {
			assertCommandContains(t, cmd, "--tag", "127.0.0.1:5512/esb-lambda-echo:e2e-test")
			buildFound = true
			break
		}
	}
	if !buildFound {
		t.Fatalf("expected function build command, got: %v", runner.commands)
	}

	imageRef := outputFunctionImageRef(t, outputDir, "lambda-echo")
	if imageRef != "127.0.0.1:5512/esb-lambda-echo:e2e-test" {
		t.Fatalf("unexpected output image ref: %s", imageRef)
	}
}

func TestExecuteEnsuresBaseWhenNoFunctionTargets(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test-no-targets",
		"127.0.0.1:5010/esb-lambda-base:e2e-test-no-targets",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	if err := os.WriteFile(functionsPath, []byte("functions: {}\n"), 0o600); err != nil {
		t.Fatalf("write functions.yml: %v", err)
	}

	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")
	t.Setenv("ESB_TAG", "latest")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

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
	if len(runner.commands) != 2 {
		t.Fatalf("expected base pull/push commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if !slices.Equal(runner.commands[0], []string{"docker", "pull", "127.0.0.1:5010/esb-lambda-base:latest"}) {
		t.Fatalf("unexpected base pull command: %v", runner.commands[0])
	}
	if !slices.Equal(runner.commands[1], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-base:latest"}) {
		t.Fatalf("unexpected base push command: %v", runner.commands[1])
	}
	if _, err := os.Stat(filepath.Join(outputDir, "functions.yml")); err != nil {
		t.Fatalf("functions.yml not merged: %v", err)
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

func outputFunctionImageRef(t *testing.T, outputDir, functionName string) string {
	t.Helper()
	path := filepath.Join(outputDir, "functions.yml")
	payload, ok, err := loadYAML(path)
	if err != nil {
		t.Fatalf("load functions.yml: %v", err)
	}
	if !ok {
		t.Fatalf("missing functions.yml: %s", path)
	}
	functionsRaw, ok := payload["functions"].(map[string]any)
	if !ok {
		t.Fatalf("functions must be map in %s", path)
	}
	entry, ok := functionsRaw[functionName].(map[string]any)
	if !ok {
		t.Fatalf("missing function %s in %s", functionName, path)
	}
	imageRaw, ok := entry["image"]
	if !ok {
		t.Fatalf("missing image for function %s in %s", functionName, path)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", imageRaw))
}
