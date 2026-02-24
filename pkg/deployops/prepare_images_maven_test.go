package deployops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
