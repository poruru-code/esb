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

func TestRewriteDockerfileForBuildRewritesNonBaseAliasStages(t *testing.T) {
	content := strings.Join([]string{
		"FROM registry:5010/esb-lambda-base:latest AS base",
		"FROM registry:5010/esb-tooling:latest AS tooling",
		"RUN echo ready",
		"FROM base",
		"",
	}, "\n")
	rewritten, changed := rewriteDockerfileForBuild(
		content,
		"127.0.0.1:5512",
		[]string{"registry:5010", "127.0.0.1:5512"},
		"runtime-v2",
	)
	if !changed {
		t.Fatal("expected dockerfile rewrite")
	}
	if !strings.Contains(rewritten, "FROM 127.0.0.1:5512/esb-lambda-base:runtime-v2 AS base") {
		t.Fatalf("expected lambda base alias+tag rewrite, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "FROM 127.0.0.1:5512/esb-tooling:latest AS tooling") {
		t.Fatalf("expected non-base alias rewrite, got:\n%s", rewritten)
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

func TestBuildxBuildCommandPropagatesProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://upper-http.example:8080")
	t.Setenv("http_proxy", "http://lower-http.example:8080")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "http://lower-https.example:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,registry")
	t.Setenv("no_proxy", "lower.local")

	cmd := buildxBuildCommand("example:latest", "/tmp/Dockerfile", "/tmp/context", false)

	assertCommandContains(t, cmd, "--build-arg", "HTTP_PROXY=http://upper-http.example:8080")
	assertCommandContains(t, cmd, "--build-arg", "http_proxy=http://upper-http.example:8080")
	assertCommandContains(t, cmd, "--build-arg", "HTTPS_PROXY=http://lower-https.example:8443")
	assertCommandContains(t, cmd, "--build-arg", "https_proxy=http://lower-https.example:8443")
	assertCommandContains(t, cmd, "--build-arg", "NO_PROXY=localhost,127.0.0.1,registry")
	assertCommandContains(t, cmd, "--build-arg", "no_proxy=localhost,127.0.0.1,registry")
}

func TestBuildxBuildCommandSkipsProxyBuildArgsWhenUnset(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	cmd := buildxBuildCommand("example:latest", "/tmp/Dockerfile", "/tmp/context", false)
	joined := strings.Join(cmd, " ")

	for _, token := range []string{
		"HTTP_PROXY=",
		"http_proxy=",
		"HTTPS_PROXY=",
		"https_proxy=",
		"NO_PROXY=",
		"no_proxy=",
	} {
		if strings.Contains(joined, token) {
			t.Fatalf("unexpected proxy build arg %q in command: %v", token, cmd)
		}
	}
}

func TestRewriteDockerfileForMavenShimRewritesMavenBaseStage(t *testing.T) {
	content := strings.Join([]string{
		"FROM maven:3.9.11-eclipse-temurin-21 AS builder",
		"WORKDIR /src",
		"RUN mvn -q -DskipTests package",
		"",
	}, "\n")
	rewritten, changed, err := rewriteDockerfileForMavenShim(content, func(baseRef string) (string, error) {
		if baseRef != "maven:3.9.11-eclipse-temurin-21" {
			t.Fatalf("unexpected base ref: %s", baseRef)
		}
		return "esb-maven-shim:deadbeef", nil
	})
	if err != nil {
		t.Fatalf("rewriteDockerfileForMavenShim() error = %v", err)
	}
	if !changed {
		t.Fatal("expected maven shim rewrite")
	}
	if !strings.Contains(rewritten, "FROM esb-maven-shim:deadbeef AS builder") {
		t.Fatalf("expected maven base rewrite, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "RUN mvn -q -DskipTests package") {
		t.Fatalf("expected mvn command preserved, got:\n%s", rewritten)
	}
}

func TestRewriteDockerfileForMavenShimFailsWithoutMavenBaseStage(t *testing.T) {
	content := strings.Join([]string{
		"FROM ubuntu:22.04 AS builder",
		"RUN mvn -q -DskipTests package",
		"",
	}, "\n")
	_, _, err := rewriteDockerfileForMavenShim(content, func(string) (string, error) {
		t.Fatal("resolver should not be called")
		return "", nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "maven run command detected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareImagesBuildsMavenShimAndRewritesFunctionDockerfile(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"maven:3.9.11-eclipse-temurin-21",
	)
	functionDockerfile := filepath.Join(root, "fixture", "functions", "lambda-echo", "Dockerfile")
	mustWriteFile(
		t,
		functionDockerfile,
		"FROM maven:3.9.11-eclipse-temurin-21 AS builder\nRUN mvn -q -DskipTests package\n",
	)
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,registry")
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")

	originalImageExists := dockerImageExistsFunc
	dockerImageExistsFunc = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExistsFunc = originalImageExists })

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
				file := cmd[i+1]
				if !strings.Contains(file, "functions/lambda-echo/Dockerfile") {
					continue
				}
				data, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				functionBuildText = string(data)
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

	shimBuildFound := false
	shimPushFound := false
	for _, cmd := range runner.commands {
		if len(cmd) >= 3 && cmd[0] == "docker" && cmd[1] == "push" && strings.HasPrefix(cmd[2], "127.0.0.1:5010/esb-maven-shim:") {
			shimPushFound = true
			continue
		}
		if len(cmd) < 4 || cmd[0] != "docker" || cmd[1] != "buildx" || cmd[2] != "build" {
			continue
		}
		if slices.Contains(cmd, "--build-arg") && strings.Contains(strings.Join(cmd, " "), "BASE_MAVEN_IMAGE=maven:3.9.11-eclipse-temurin-21") {
			shimBuildFound = true
			assertCommandContains(t, cmd, "--tag", "127.0.0.1:5010/esb-maven-shim:0f9e5ac6f33b3755")
			assertCommandContains(t, cmd, "--build-arg", "HTTP_PROXY=http://proxy.example:8080")
			assertCommandContains(t, cmd, "--build-arg", "http_proxy=http://proxy.example:8080")
		}
	}
	if !shimBuildFound {
		t.Fatalf("expected maven shim build command, got: %v", runner.commands)
	}
	if !shimPushFound {
		t.Fatalf("expected maven shim push command, got: %v", runner.commands)
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-maven-shim:") {
		t.Fatalf("expected function dockerfile to use maven shim image, got:\n%s", functionBuildText)
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
