package engine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("PrepareImages() error = %v", err)
	}
	if len(runner.commands) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(runner.commands))
	}

	if runner.commands[0][0:3][0] != "docker" || runner.commands[0][1] != "buildx" || runner.commands[0][2] != "build" {
		t.Fatalf("unexpected base build command: %v", runner.commands[0])
	}
	wantBaseDockerfile := filepath.Join(root, "fixture", "runtime-base", "runtime-hooks", "python", "docker", "Dockerfile")
	wantBaseContext := filepath.Join(root, "fixture", "runtime-base")
	assertCommandContains(t, runner.commands[0], "--tag", "127.0.0.1:5010/esb-lambda-base:e2e-test")
	assertCommandContains(t, runner.commands[0], "--file", wantBaseDockerfile)
	if got := runner.commands[0][len(runner.commands[0])-1]; got != wantBaseContext {
		t.Fatalf("base build context = %q, want %q", got, wantBaseContext)
	}
	if !slices.Equal(runner.commands[1], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-base:e2e-test"}) {
		t.Fatalf("unexpected base push command: %v", runner.commands[1])
	}

	if runner.commands[2][0:3][0] != "docker" || runner.commands[2][1] != "buildx" || runner.commands[2][2] != "build" {
		t.Fatalf("unexpected function build command: %v", runner.commands[2])
	}
	assertCommandContains(t, runner.commands[2], "--tag", "127.0.0.1:5010/esb-lambda-echo:e2e-test")
	if !slices.Equal(runner.commands[3], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-echo:e2e-test"}) {
		t.Fatalf("unexpected function push command: %v", runner.commands[3])
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
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("PrepareImages() error = %v", err)
	}
	if len(runner.commands) != 6 {
		t.Fatalf("expected 6 commands, got %d", len(runner.commands))
	}

	assertCommandContains(t, runner.commands[0], "--tag", "127.0.0.1:5010/esb-lambda-base:e2e-test")
	if !slices.Equal(runner.commands[1], []string{
		"docker",
		"tag",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
		"registry:5010/esb-lambda-base:e2e-test",
	}) {
		t.Fatalf("unexpected base tag command: %v", runner.commands[1])
	}
	if !slices.Equal(runner.commands[2], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-base:e2e-test"}) {
		t.Fatalf("unexpected base push command: %v", runner.commands[2])
	}

	assertCommandContains(t, runner.commands[3], "--tag", "registry:5010/esb-lambda-echo:e2e-test")
	if !slices.Equal(runner.commands[4], []string{
		"docker",
		"tag",
		"registry:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
	}) {
		t.Fatalf("unexpected function tag command: %v", runner.commands[4])
	}
	if !slices.Equal(runner.commands[5], []string{"docker", "push", "127.0.0.1:5010/esb-lambda-echo:e2e-test"}) {
		t.Fatalf("unexpected function push command: %v", runner.commands[5])
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

	if err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	}); err != nil {
		t.Fatalf("PrepareImages() error = %v", err)
	}

	if functionBuildFile == "" {
		t.Fatal("expected function build dockerfile capture")
	}
	if !strings.HasSuffix(functionBuildFile, ".artifactctl.build") {
		t.Fatalf("expected temporary dockerfile suffix, got: %s", functionBuildFile)
	}
	if !strings.Contains(functionBuildText, "FROM 127.0.0.1:5010/esb-lambda-base:e2e-test") {
		t.Fatalf("expected rewritten build dockerfile, got:\n%s", functionBuildText)
	}
	if _, err := os.Stat(functionBuildFile); !os.IsNotExist(err) {
		t.Fatalf("expected temporary dockerfile cleanup, stat err = %v", err)
	}
}

func TestPrepareImagesTemporarilyRewritesDockerignore(t *testing.T) {
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
	inspected := ""
	runner := &recordCommandRunner{
		hook: func(cmd []string) error {
			if len(cmd) > 0 && cmd[0] == "docker" && slices.Contains(cmd, "--file") {
				for i := 0; i+1 < len(cmd); i++ {
					if cmd[i] == "--file" && strings.HasSuffix(cmd[i+1], "functions/lambda-echo/Dockerfile") {
						data, err := os.ReadFile(dockerignore)
						if err != nil {
							return err
						}
						inspected = string(data)
					}
				}
			}
			return nil
		},
	}
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("PrepareImages() error = %v", err)
	}
	if inspected == "" {
		t.Fatalf("expected dockerignore inspection during function build")
	}
	if !containsLine(inspected, "!functions/lambda-echo/") || !containsLine(inspected, "!functions/lambda-echo/**") {
		t.Fatalf("expected temporary dockerignore to include lambda-echo paths, got:\n%s", inspected)
	}
	restored, err := os.ReadFile(dockerignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("expected dockerignore restoration, got:\n%s", string(restored))
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
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		NoCache:      true,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("PrepareImages() error = %v", err)
	}
	if len(runner.commands) == 0 {
		t.Fatal("expected build commands")
	}
	if !slices.Contains(runner.commands[0], "--no-cache") {
		t.Fatalf("expected --no-cache in base build command: %v", runner.commands[0])
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
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPrepareImagesFailsWhenRuntimeBaseContextMissing(t *testing.T) {
	root := t.TempDir()
	manifestPath := writePrepareImageFixture(
		t,
		root,
		"127.0.0.1:5010/esb-lambda-echo:e2e-test",
		"127.0.0.1:5010/esb-lambda-base:e2e-test",
	)
	missingDockerfile := filepath.Join(root, "fixture", "runtime-base", "runtime-hooks", "python", "docker", "Dockerfile")
	if err := os.Remove(missingDockerfile); err != nil {
		t.Fatalf("remove runtime base dockerfile: %v", err)
	}

	runner := &recordCommandRunner{}
	err := PrepareImages(PrepareImagesRequest{
		ArtifactPath: manifestPath,
		Runner:       runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "runtime base dockerfile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writePrepareImageFixture(t *testing.T, root, imageRef, baseRef string) string {
	t.Helper()
	artifactRoot := filepath.Join(root, "fixture")
	functionDir := filepath.Join(artifactRoot, "functions", "lambda-echo")
	configDir := filepath.Join(artifactRoot, "config")
	runtimeBaseDir := filepath.Join(artifactRoot, "runtime-base", "runtime-hooks", "python")
	mustMkdirAll(t, functionDir)
	mustMkdirAll(t, configDir)
	mustMkdirAll(t, filepath.Join(runtimeBaseDir, "docker"))
	mustMkdirAll(t, filepath.Join(runtimeBaseDir, "sitecustomize", "site-packages"))
	mustMkdirAll(t, filepath.Join(runtimeBaseDir, "trace-bridge", "layer"))
	mustWriteFile(
		t,
		filepath.Join(runtimeBaseDir, "docker", "Dockerfile"),
		"FROM public.ecr.aws/lambda/python:3.12\nCOPY runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py /opt/python/sitecustomize.py\nCOPY runtime-hooks/python/trace-bridge/layer/ /opt/python/\n",
	)
	mustWriteFile(t, filepath.Join(runtimeBaseDir, "sitecustomize", "site-packages", "sitecustomize.py"), "# stub")
	mustWriteFile(t, filepath.Join(runtimeBaseDir, "trace-bridge", "layer", "trace_bridge.py"), "# stub")
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

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-e2e-docker",
		Env:           "e2e-docker",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "fixture",
				RuntimeConfigDir: "config",
				SourceTemplate: ArtifactSourceTemplate{
					Path:   "e2e/fixtures/template.e2e.yaml",
					SHA256: "sha",
				},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID(
		manifest.Artifacts[0].SourceTemplate.Path,
		manifest.Artifacts[0].SourceTemplate.Parameters,
		manifest.Artifacts[0].SourceTemplate.SHA256,
	)
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
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

func containsLine(content, needle string) bool {
	for _, line := range strings.Split(content, "\n") {
		if line == needle {
			return true
		}
	}
	return false
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
