package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type recordCommandRunner struct {
	commands [][]string
	hook     func([]string) error
}

func (r *recordCommandRunner) Run(cmd []string) error {
	clone := append([]string(nil), cmd...)
	r.commands = append(r.commands, clone)
	if r.hook != nil {
		return r.hook(clone)
	}
	return nil
}

func TestPrepareImagesBuildsAndPushesDockerRefs(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(runner.commands))
	}

	if runner.commands[0][0:3][0] != "docker" || runner.commands[0][1] != "buildx" || runner.commands[0][2] != "build" {
		t.Fatalf("unexpected function build command: %v", runner.commands[0])
	}
	assertCommandContains(t, runner.commands[0], "--tag", "127.0.0.1:5010/esb-lambda-echo:e2e-test")
	if !slices.Equal(runner.commands[1], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-echo:e2e-test"}) {
		t.Fatalf("unexpected function push command: %v", runner.commands[1])
	}
}

func TestPrepareImagesNormalizesFixedArtifactRegistryToRuntimeRegistry(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "127.0.0.1:5512")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5512")

	var functionBuildText string
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) < 4 || cmd[0] != "docker" || cmd[1] != "buildx" || cmd[2] != "build" {
				return nil
			}
			for i := 0; i+1 < len(cmd); i++ {
				if cmd[i] != "--file" {
					continue
				}
				data, err := os.ReadFile(cmd[i+1])
				if err != nil {
					return err
				}
				functionBuildText = string(data)
				return nil
			}
			return nil
		},
	}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected function build/push commands, got %d: %v", len(runner.commands), runner.commands)
	}
	assertCommandContains(t, runner.commands[0], "--tag", "127.0.0.1:5512/esb-lambda-echo:e2e-test")
	if !slices.Equal(runner.commands[1], []string{"docker", "push", "127.0.0.1:5512/esb-lambda-echo:e2e-test"}) {
		t.Fatalf("unexpected function push command: %v", runner.commands[1])
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5512/esb-lambda-base:e2e-test") {
		t.Fatalf("expected rewritten build dockerfile, got:\n%s", functionBuildText)
	}
}

func TestPrepareImagesRewritesPushTargetsForContainerdRefs(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:e2e-test",
		"registry:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(runner.commands))
	}

	assertCommandContains(t, runner.commands[0], "--tag", "registry:5010/esb-lambda-echo:e2e-test")
	if !slices.Equal(runner.commands[1], []string{
		"docker",
		"tag",
		"registry:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
	}) {
		t.Fatalf("unexpected function tag command: %v", runner.commands[1])
	}
	if !slices.Equal(runner.commands[2], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-echo:e2e-test"}) {
		t.Fatalf("unexpected function push command: %v", runner.commands[2])
	}
}

func TestPrepareImagesRewritesFunctionDockerfileRegistryForBuild(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:e2e-test",
		"registry:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	var (
		functionBuildFile string
		functionBuildText string
	)
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) < 4 || cmd[0] != "docker" || cmd[1] != "buildx" || cmd[2] != "build" {
				return nil
			}
			for i := 0; i+1 < len(cmd); i++ {
				if cmd[i] != "--file" {
					continue
				}
				file := cmd[i+1]
				if !strings.Contains(file, "functions/lambda-echo/Dockerfile") {
					continue
				}
				functionBuildFile = file
				data, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				functionBuildText = string(data)
				break
			}
			return nil
		},
	}

	if err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	}); err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}

	if functionBuildFile == "" {
		t.Fatal("expected function build dockerfile capture")
	}
	if !strings.HasSuffix(functionBuildFile, ".artifact.build") {
		t.Fatalf("expected temporary dockerfile suffix, got: %s", functionBuildFile)
	}
	artifactRoot := filepath.Join(root, "fixture")
	if strings.HasPrefix(functionBuildFile, artifactRoot+string(os.PathSeparator)) {
		t.Fatalf("temporary dockerfile must be outside artifact root: %s", functionBuildFile)
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-lambda-base:e2e-test") {
		t.Fatalf("expected rewritten build dockerfile, got:\n%s", functionBuildText)
	}
	if _, err := os.Stat(functionBuildFile); !os.IsNotExist(err) {
		t.Fatalf("expected temporary dockerfile cleanup, stat err = %v", err)
	}
}

func TestPrepareImagesRewritesFunctionDockerfileLambdaBaseTagFromRuntimeObservation(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:e2e-test",
		"registry:5010/esb-lambda-base:latest",
	)
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	var functionBuildText string
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) < 4 || cmd[0] != "docker" || cmd[1] != "buildx" || cmd[2] != "build" {
				return nil
			}
			for i := 0; i+1 < len(cmd); i++ {
				if cmd[i] != "--file" {
					continue
				}
				data, err := os.ReadFile(cmd[i+1])
				if err != nil {
					return err
				}
				functionBuildText = string(data)
				return nil
			}
			return nil
		},
	}

	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		Runtime: &artifactcore.RuntimeObservation{
			ESBVersion: "runtime-v2",
		},
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-lambda-base:runtime-v2") {
		t.Fatalf("expected runtime tag rewrite in build dockerfile, got:\n%s", functionBuildText)
	}
}

func TestPrepareImagesDoesNotRewritePinnedLambdaBaseTagFromRuntimeObservation(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:e2e-test",
		"registry:5010/esb-lambda-base:e2e-test",
	)
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	var functionBuildText string
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) < 4 || cmd[0] != "docker" || cmd[1] != "buildx" || cmd[2] != "build" {
				return nil
			}
			for i := 0; i+1 < len(cmd); i++ {
				if cmd[i] != "--file" {
					continue
				}
				data, err := os.ReadFile(cmd[i+1])
				if err != nil {
					return err
				}
				functionBuildText = string(data)
				return nil
			}
			return nil
		},
	}

	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		Runtime: &artifactcore.RuntimeObservation{
			ESBVersion: "runtime-v2",
		},
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-lambda-base:e2e-test") {
		t.Fatalf("expected pinned tag to be preserved in build dockerfile, got:\n%s", functionBuildText)
	}
}

func TestPrepareImagesLeavesArtifactRootUnchanged(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	artifactRoot := filepath.Join(root, "fixture")
	dockerignore := filepath.Join(artifactRoot, ".dockerignore")
	original := "*\n!.dockerignore\n!functions/\n!functions/lambda-scheduled/\n!functions/lambda-scheduled/**\n"
	if err := os.WriteFile(dockerignore, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	unchanged, err := os.ReadFile(dockerignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != original {
		t.Fatalf("artifact root file changed unexpectedly: got:\n%s", string(unchanged))
	}
	err = filepath.WalkDir(artifactRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".artifact.build") {
			t.Fatalf("artifact root should not contain temporary build files: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk artifact root: %v", err)
	}
	functionBuildContext := ""
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[0] == "docker" && cmd[1] == "buildx" && cmd[2] == "build" {
			if slices.Contains(cmd, "--file") && slices.Contains(cmd, "--tag") && strings.Contains(strings.Join(cmd, " "), "esb-lambda-echo:e2e-test") {
				functionBuildContext = cmd[len(cmd)-1]
				break
			}
		}
	}
	if functionBuildContext == "" {
		t.Fatal("expected function build command")
	}
	if functionBuildContext == artifactRoot || strings.HasPrefix(functionBuildContext, artifactRoot+string(os.PathSeparator)) {
		t.Fatalf("function build must not use artifact root as context: %s", functionBuildContext)
	}
}

func TestPrepareImagesNoCacheAddsFlag(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		NoCache:      true,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) == 0 {
		t.Fatal("expected build commands")
	}
	if !slices.Contains(runner.commands[0], "--no-cache") {
		t.Fatalf("expected --no-cache in base build command: %v", runner.commands[0])
	}
}

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
	t.Setenv("ESB_TAG", "latest")

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
	t.Setenv("ESB_TAG", "latest")

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
	t.Setenv("ESB_TAG", "latest")

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
	t.Setenv("ESB_TAG", "latest")

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

func TestPrepareImagesReturnsRunnerError(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	runner := &recordCommandRunner{
		hook: func(_ []string) error { return errors.New("boom") },
	}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPrepareImagesRequiresArtifactPath(t *testing.T) {
	err := prepareImages(prepareImagesInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "artifact path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareImagesFailsWhenFunctionsIsNotMap(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions: []\n")

	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       &recordCommandRunner{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "functions must be map") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareImagesSkipsWhenNoImageTarget(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	functionsPath := filepath.Join(root, "fixture", "config", "functions.yml")
	mustWriteFile(t, functionsPath, "functions:\n  lambda-echo:\n    handler: index.handler\n")

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("expected no docker command, got %v", runner.commands)
	}
}

func TestDefaultCommandRunnerRejectsEmptyCommand(t *testing.T) {
	runner := defaultCommandRunner{}
	err := runner.Run(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "command is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writePrepareImageFixture(t *testing.T, root, imageRef, baseRef string) string {
	t.Helper()
	artifactRoot := filepath.Join(root, "fixture")
	functionDir := filepath.Join(artifactRoot, "functions", "lambda-echo")
	configDir := filepath.Join(artifactRoot, "config")
	mustMkdirAll(t, functionDir)
	mustMkdirAll(t, configDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM "+baseRef+"\nCOPY functions/lambda-echo/src/ /var/task/\n",
	)
	mustMkdirAll(t, filepath.Join(functionDir, "src"))
	mustWriteFile(t, filepath.Join(functionDir, "src", "lambda_function.py"), "def lambda_handler(event, context):\n    return {'ok': True}\n")
	mustWriteFile(
		t,
		filepath.Join(configDir, "functions.yml"),
		"functions:\n  lambda-echo:\n    image: \""+imageRef+"\"\n",
	)
	mustWriteFile(t, filepath.Join(configDir, "routing.yml"), "routes: []\n")
	mustWriteFile(t, filepath.Join(configDir, "resources.yml"), "resources: {}\n")

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       "esb-e2e-docker",
		Env:           "e2e-docker",
		Mode:          "docker",
		Artifacts: []artifactcore.ArtifactEntry{
			{
				ArtifactRoot:     "fixture",
				RuntimeConfigDir: "config",
			},
		},
	}
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func assertCommandContains(t *testing.T, cmd []string, key, value string) {
	t.Helper()
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == key && cmd[i+1] == value {
			return
		}
	}
	t.Fatalf("command missing %s %s: %v", key, value, cmd)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
