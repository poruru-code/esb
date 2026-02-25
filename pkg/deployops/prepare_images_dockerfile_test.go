package deployops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRewriteDockerfileForBuildRewritesAliasStages(t *testing.T) {
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
	)
	if !changed {
		t.Fatal("expected dockerfile rewrite")
	}
	if !strings.Contains(rewritten, "FROM 127.0.0.1:5512/esb-lambda-base:latest AS base") {
		t.Fatalf("expected lambda base alias rewrite, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "FROM 127.0.0.1:5512/esb-tooling:latest AS tooling") {
		t.Fatalf("expected non-base alias rewrite, got:\n%s", rewritten)
	}
}

func TestPrepareImagesDoesNotRewriteLambdaBaseTagFromESBTagEnv(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"registry:5010/esb-lambda-echo:e2e-test",
		"registry:5010/esb-lambda-base:latest",
	)
	t.Setenv("CONTAINER_REGISTRY", "registry:5010")
	t.Setenv("HOST_REGISTRY_ADDR", "127.0.0.1:5010")
	t.Setenv("ESB_TAG", "env-tag-should-not-apply")

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
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-lambda-base:latest") {
		t.Fatalf("expected latest tag to remain when runtime observation is absent, got:\n%s", functionBuildText)
	}
	if strings.Contains(functionBuildText, "env-tag-should-not-apply") {
		t.Fatalf("unexpected ESB_TAG env rewrite, got:\n%s", functionBuildText)
	}
}
