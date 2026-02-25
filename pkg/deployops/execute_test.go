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
		ArtifactPath:     "",
		RuntimeConfigDir: filepath.Join(t.TempDir(), "runtime-config"),
		Runner:           runner,
	})
	if !errors.Is(err, artifactcore.ErrArtifactPathRequired) {
		t.Fatalf("expected ErrArtifactPathRequired, got %v", err)
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
		ArtifactPath:     manifestPath,
		RuntimeConfigDir: outputDir,
		Runner:           runner,
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
		ArtifactPath:     manifestPath,
		RuntimeConfigDir: outputDir,
		Runner:           runner,
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

func TestExecuteDoesNotNormalizeOutputImageWithoutPublishedFunctionImage(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	dockerfilePath := filepath.Join(root, "fixture", "functions", "lambda-echo", "Dockerfile")
	if err := os.Remove(dockerfilePath); err != nil {
		t.Fatalf("remove dockerfile: %v", err)
	}

	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5512")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5512")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return true }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	outputDir := filepath.Join(root, "out")
	runner := &recordCommandRunner{}
	result, err := Execute(Input{
		ArtifactPath:     manifestPath,
		RuntimeConfigDir: outputDir,
		Runner:           runner,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}

	imageRef := outputFunctionImageRef(t, outputDir, "lambda-echo")
	if imageRef != "127.0.0.1:5010/esb-lambda-echo:e2e-test" {
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
	t.Setenv("ESB_TAG", "unexpected-env-tag")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	outputDir := filepath.Join(root, "out")
	runner := &recordCommandRunner{}
	result, err := Execute(Input{
		ArtifactPath:     manifestPath,
		RuntimeConfigDir: outputDir,
		Runner:           runner,
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
		ArtifactPath:     "  artifact.yml  ",
		RuntimeConfigDir: "  out  ",
	})
	if err != nil {
		t.Fatalf("normalizeInput error = %v", err)
	}
	if normalized.ArtifactPath != "artifact.yml" || normalized.RuntimeConfigDir != "out" {
		t.Fatalf("unexpected normalized input: %#v", normalized)
	}
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
