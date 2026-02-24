package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fixtureRecordRunner struct {
	commands [][]string
}

func (r *fixtureRecordRunner) Run(cmd []string) error {
	r.commands = append(r.commands, append([]string(nil), cmd...))
	return nil
}

func mustWriteFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeFixtureManifest(t *testing.T, root, artifactRoot string) string {
	t.Helper()
	manifestPath := filepath.Join(root, "artifact.yml")
	content := strings.Join([]string{
		`schema_version: "1"`,
		`project: esb`,
		`env: e2e-docker`,
		`mode: docker`,
		`artifacts:`,
		`  - artifact_root: ` + artifactRoot,
		`    runtime_config_dir: config`,
		"",
	}, "\n")
	mustWriteFixtureFile(t, manifestPath, content)
	return manifestPath
}

func assertFixtureCommandContains(t *testing.T, cmd []string, key, expected string) {
	t.Helper()
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == key && cmd[i+1] == expected {
			return
		}
	}
	t.Fatalf("expected command to contain %s %s, got: %v", key, expected, cmd)
}

func TestExecuteFixtureImageEnsureBuildsPythonFixture(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	fixtureRoot := filepath.Join(root, "fixtures")
	mustWriteFixtureFile(
		t,
		filepath.Join(artifactRoot, "functions", "lambda-image", "Dockerfile"),
		"FROM 127.0.0.1:5010/esb-e2e-image-python:latest\n",
	)
	mustWriteFixtureFile(t, filepath.Join(fixtureRoot, "python", "Dockerfile"), "FROM public.ecr.aws/lambda/python:3.12\n")
	manifestPath := writeFixtureManifest(t, root, artifactRoot)

	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	runner := &fixtureRecordRunner{}

	result, err := executeFixtureImageEnsureWithLogWriter(FixtureImageEnsureInput{
		ArtifactPath: manifestPath,
		FixtureRoot:  fixtureRoot,
		Runner:       runner,
	}, nil)
	if err != nil {
		t.Fatalf("executeFixtureImageEnsureWithLogWriter() error = %v", err)
	}
	if result.SchemaVersion != fixtureImageEnsureSchemaVersion {
		t.Fatalf("unexpected schema version: %#v", result)
	}
	if !slices.Equal(result.PreparedImages, []string{"127.0.0.1:5010/esb-e2e-image-python:latest"}) {
		t.Fatalf("unexpected prepared images: %#v", result.PreparedImages)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected build+push commands, got: %v", runner.commands)
	}
	if !slices.Equal(runner.commands[1], []string{"docker", "push", "127.0.0.1:5010/esb-e2e-image-python:latest"}) {
		t.Fatalf("unexpected push command: %v", runner.commands[1])
	}
	buildCmd := runner.commands[0]
	if len(buildCmd) < 4 || !slices.Equal(buildCmd[0:3], []string{"docker", "buildx", "build"}) {
		t.Fatalf("unexpected build command: %v", buildCmd)
	}
	assertFixtureCommandContains(t, buildCmd, "--build-arg", "HTTP_PROXY=http://proxy.example:8080")
	assertFixtureCommandContains(t, buildCmd, "--build-arg", "http_proxy=http://proxy.example:8080")
}

func TestExecuteFixtureImageEnsureBuildsJavaFixtureWithMavenShim(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	fixtureRoot := filepath.Join(root, "fixtures")
	mustWriteFixtureFile(
		t,
		filepath.Join(artifactRoot, "functions", "lambda-image", "Dockerfile"),
		"FROM 127.0.0.1:5010/esb-e2e-image-java:latest\n",
	)
	mustWriteFixtureFile(
		t,
		filepath.Join(fixtureRoot, "java", "Dockerfile"),
		strings.Join([]string{
			"ARG MAVEN_IMAGE=public.ecr.aws/sam/build-java21:latest",
			"FROM ${MAVEN_IMAGE} AS builder",
			"RUN mvn -q -DskipTests package",
			"",
		}, "\n"),
	)
	manifestPath := writeFixtureManifest(t, root, artifactRoot)

	runner := &fixtureRecordRunner{}
	var gotEnsureInput MavenShimEnsureInput
	result, err := executeFixtureImageEnsureWithLogWriter(FixtureImageEnsureInput{
		ArtifactPath: manifestPath,
		FixtureRoot:  fixtureRoot,
		Runner:       runner,
		EnsureMavenShim: func(input MavenShimEnsureInput) (MavenShimEnsureResult, error) {
			gotEnsureInput = input
			return MavenShimEnsureResult{
				SchemaVersion: 1,
				ShimImage:     "127.0.0.1:5010/esb-maven-shim:deadbeefdeadbeef",
			}, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("executeFixtureImageEnsureWithLogWriter() error = %v", err)
	}
	if !slices.Equal(result.PreparedImages, []string{"127.0.0.1:5010/esb-e2e-image-java:latest"}) {
		t.Fatalf("unexpected prepared images: %#v", result.PreparedImages)
	}
	if gotEnsureInput.BaseImage != javaFixtureMavenBaseImage {
		t.Fatalf("unexpected maven base image: %#v", gotEnsureInput)
	}
	if gotEnsureInput.HostRegistry != "127.0.0.1:5010" {
		t.Fatalf("unexpected host registry: %#v", gotEnsureInput)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected build+push commands, got: %v", runner.commands)
	}
	buildCmd := runner.commands[0]
	assertFixtureCommandContains(
		t,
		buildCmd,
		"--build-arg",
		"MAVEN_IMAGE=127.0.0.1:5010/esb-maven-shim:deadbeefdeadbeef",
	)
}

func TestExecuteFixtureImageEnsureSkipsWhenNoFixtureSources(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifact")
	mustWriteFixtureFile(
		t,
		filepath.Join(artifactRoot, "functions", "lambda-echo", "Dockerfile"),
		"FROM public.ecr.aws/lambda/python:3.12\n",
	)
	manifestPath := writeFixtureManifest(t, root, artifactRoot)
	runner := &fixtureRecordRunner{}

	result, err := executeFixtureImageEnsureWithLogWriter(FixtureImageEnsureInput{
		ArtifactPath: manifestPath,
		Runner:       runner,
		FixtureRoot:  filepath.Join(root, "fixtures"),
	}, nil)
	if err != nil {
		t.Fatalf("executeFixtureImageEnsureWithLogWriter() error = %v", err)
	}
	if len(result.PreparedImages) != 0 {
		t.Fatalf("expected no prepared images, got: %#v", result.PreparedImages)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("expected no commands, got: %v", runner.commands)
	}
}
