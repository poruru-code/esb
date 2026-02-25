package deployops

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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

func TestPrepareImagesAddsLayerBuildContextsForZipLayers(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".branding.env"), "export BRANDING_SLUG=esb\n")
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)

	functionDir := filepath.Join(root, "fixture", "functions", "lambda-echo")
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM 127.0.0.1:5010/esb-lambda-base:e2e-test\n"+
			"ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}\n"+
			"COPY --from=layer_0_zip-layer / /opt/\n"+
			"COPY functions/lambda-echo/src/ /var/task/\n",
	)
	writeZipArchive(
		t,
		filepath.Join(functionDir, "layers", "zip-layer.zip"),
		map[string]string{"lib_zip.py": "print('zip')\n"},
	)

	runner := &recordCommandRunner{}
	err := prepareImages(prepareImagesInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("prepareImages() error = %v", err)
	}

	buildCmd := findFunctionBuildCommand(t, runner.commands)
	contextPath := findCommandValue(buildCmd, "--build-context", "layer_0_zip-layer=")
	if contextPath == "" {
		t.Fatalf("expected --build-context for zip layer, got %v", buildCmd)
	}
	if !strings.HasPrefix(filepath.Clean(contextPath), filepath.Clean(filepath.Join(root, ".esb", "cache", "layers"))) {
		t.Fatalf("unexpected cache path: %s", contextPath)
	}
	if _, err := os.Stat(filepath.Join(contextPath, "python", "lib_zip.py")); err != nil {
		t.Fatalf("expected extracted layer content in python dir: %v", err)
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

func findFunctionBuildCommand(t *testing.T, commands [][]string) []string {
	t.Helper()
	for _, cmd := range commands {
		if len(cmd) < 4 {
			continue
		}
		if cmd[0] == "docker" && cmd[1] == "buildx" && cmd[2] == "build" {
			return cmd
		}
	}
	t.Fatalf("function build command not found: %v", commands)
	return nil
}

func findCommandValue(cmd []string, key, valuePrefix string) string {
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] != key {
			continue
		}
		value := strings.TrimSpace(cmd[i+1])
		if strings.HasPrefix(value, valuePrefix) {
			return strings.TrimPrefix(value, valuePrefix)
		}
	}
	return ""
}
