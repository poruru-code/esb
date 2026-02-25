package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPrepareImagesDefaultRunnerDoesNotAutoEnsureBase(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:unit-default-runner",
		"127.0.0.1:5010/esb-lambda-base:unit-default-runner",
	)
	runner := &recordCommandRunner{}

	originalFactory := defaultCommandRunnerFactory
	defaultCommandRunnerFactory = func() CommandRunner { return runner }
	t.Cleanup(func() { defaultCommandRunnerFactory = originalFactory })

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	err = prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected only function build/push commands, got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestPrepareImagesEnsureBaseRequiresRuntimeHooksDockerfile(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:unit-ensure-base",
		"127.0.0.1:5010/esb-lambda-base:unit-ensure-base",
	)
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) >= 2 && cmd[0] == "docker" && cmd[1] == "pull" {
				return errors.New("pull failed")
			}
			return nil
		},
	}

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	err = prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		EnsureBase:   true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "runtime hooks dockerfile is unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareImagesEnsureBaseRunsWithoutFunctionTargets(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:unit-ensure-base-no-targets",
		"127.0.0.1:5010/esb-lambda-base:unit-ensure-base-no-targets",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions: {}\n")

	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")
	t.Setenv("ESB_TAG", "unexpected-env-tag")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		EnsureBase:   true,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
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
}

func TestPrepareImagesEnsureBasePushesWhenBaseExistsLocally(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:unit-ensure-base-local-present",
		"registry:5010/esb-lambda-base:unit-ensure-base-local-present",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions: {}\n")

	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")
	t.Setenv("ESB_TAG", "unexpected-env-tag")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(ref string) bool {
		return ref == "127.0.0.1:5010/esb-lambda-base:latest"
	}
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		EnsureBase:   true,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected base push command, got %d: %v", len(runner.commands), runner.commands)
	}
	if !slices.Equal(runner.commands[0], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-base:latest"}) {
		t.Fatalf("unexpected base push command: %v", runner.commands[0])
	}
}

func TestPrepareImagesEnsureBasePullsBeforeBuildingWhenLocalMissing(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:unit-ensure-base-pull-first",
		"127.0.0.1:5010/esb-lambda-base:unit-ensure-base-pull-first",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions: {}\n")

	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")
	t.Setenv("ESB_TAG", "unexpected-env-tag")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		EnsureBase:   true,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
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
}

func TestPrepareImagesEnsureBaseWithoutTargetsRequiresRegistryEnv(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:unit-ensure-base-requires-registry",
		"127.0.0.1:5010/esb-lambda-base:unit-ensure-base-requires-registry",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions: {}\n")

	t.Setenv("CONTAINER_REGISTRY", "")
	t.Setenv("HOST_REGISTRY_ADDR", "")
	t.Setenv("REGISTRY", "")

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		EnsureBase:   true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lambda base registry is unresolved") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("expected no docker command when registry is unresolved, got: %v", runner.commands)
	}
}
