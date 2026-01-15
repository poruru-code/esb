// Where: cli/internal/generator/go_builder_test.go
// What: Tests for GoBuilder build workflow.
// Why: Ensure Go-based build wiring matches expected config/output behavior.
package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/app"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func TestGoBuilderBuildGeneratesAndBuilds(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	cfg := config.GeneratorConfig{
		App: config.AppConfig{Name: "demo"},
		Environments: config.Environments{
			{Name: "staging", Mode: "containerd"},
		},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
	}
	if err := config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg); err != nil {
		t.Fatalf("write generator.yml: %v", err)
	}

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.containerd.yml",
	)
	// Create mock service directories and root files required by staging logic
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "gateway"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "pyproject.toml"), "[project]\n")
	writeTestFile(t, filepath.Join(repoRoot, "cli", "internal", "generator", "assets", "Dockerfile.lambda-base"), "FROM scratch\n")

	var gotCfg config.GeneratorConfig
	var gotOpts GenerateOptions
	generate := func(cfg config.GeneratorConfig, opts GenerateOptions) ([]FunctionSpec, error) {
		gotCfg = cfg
		gotOpts = opts

		outputDir := cfg.Paths.OutputDir
		writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}")
		writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []FunctionSpec{{Name: "hello"}}, nil
	}

	dockerRunner := &recordRunner{}
	composeRunner := &recordRunner{}
	var buildOpts compose.BuildOptions

	builder := &GoBuilder{
		Runner:        dockerRunner,
		ComposeRunner: composeRunner,
		BuildCompose: func(_ context.Context, _ compose.CommandRunner, opts compose.BuildOptions) error {
			buildOpts = opts
			return nil
		},
		Generate:     generate,
		FindRepoRoot: func(string) (string, error) { return repoRoot, nil },
	}

	t.Setenv("ESB_MODE", "")
	t.Setenv("ESB_CONFIG_DIR", "")
	t.Setenv("ESB_PROJECT_NAME", "")
	t.Setenv("ESB_IMAGE_TAG", "")

	setupRootCA(t)
	request := app.BuildRequest{
		ProjectDir:   projectDir,
		TemplatePath: templatePath,
		Env:          "staging",
	}
	if err := builder.Build(request); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedOutput := filepath.Join(projectDir, ".esb", "staging")
	if gotCfg.Paths.OutputDir != expectedOutput {
		t.Fatalf("unexpected output dir: %s", gotCfg.Paths.OutputDir)
	}
	if gotCfg.Paths.SamTemplate != templatePath {
		t.Fatalf("unexpected template path: %s", gotCfg.Paths.SamTemplate)
	}
	if gotOpts.ProjectRoot != repoRoot {
		t.Fatalf("unexpected project root: %s", gotOpts.ProjectRoot)
	}
	if gotOpts.RegistryExternal != "localhost:5010" {
		t.Fatalf("unexpected registry external: %s", gotOpts.RegistryExternal)
	}
	if gotOpts.RegistryInternal != "registry:5010" {
		t.Fatalf("unexpected registry internal: %s", gotOpts.RegistryInternal)
	}
	if gotOpts.Tag != "staging" {
		t.Fatalf("unexpected tag: %s", gotOpts.Tag)
	}
	if gotOpts.Parameters["S3_ENDPOINT_HOST"] != "s3-storage" {
		t.Fatalf("missing S3_ENDPOINT_HOST parameter")
	}
	if gotOpts.Parameters["DYNAMODB_ENDPOINT_HOST"] != "database" {
		t.Fatalf("missing DYNAMODB_ENDPOINT_HOST parameter")
	}

	if buildOpts.RootDir != repoRoot {
		t.Fatalf("unexpected compose root: %s", buildOpts.RootDir)
	}
	if buildOpts.Project != "demo-staging" {
		t.Fatalf("unexpected compose project: %s", buildOpts.Project)
	}
	if buildOpts.Mode != "containerd" {
		t.Fatalf("unexpected compose mode: %s", buildOpts.Mode)
	}
	if buildOpts.Target != "control" {
		t.Fatalf("unexpected compose target: %s", buildOpts.Target)
	}
	expectedServices := []string{"os-base", "python-base", "gateway", "agent"}
	if len(buildOpts.Services) != len(expectedServices) {
		t.Fatalf("unexpected compose services: %v", buildOpts.Services)
	}
	for i, service := range expectedServices {
		if buildOpts.Services[i] != service {
			t.Fatalf("unexpected compose services: %v", buildOpts.Services)
		}
	}

	if got := os.Getenv("ESB_CONFIG_DIR"); got != "services/gateway/.esb-staging/demo-staging/staging/config" {
		t.Fatalf("unexpected ESB_CONFIG_DIR: %s", got)
	}
	if got := os.Getenv("ESB_PROJECT_NAME"); got != "demo-staging" {
		t.Fatalf("unexpected ESB_PROJECT_NAME: %s", got)
	}
	if got := os.Getenv("ESB_IMAGE_TAG"); got != "staging" {
		t.Fatalf("unexpected ESB_IMAGE_TAG: %s", got)
	}
	if got := os.Getenv("ESB_MODE"); got != "containerd" {
		t.Fatalf("unexpected ESB_MODE: %s", got)
	}

	staged := filepath.Join(repoRoot, "services", "gateway", ".esb-staging", "demo-staging", "staging", "config", "functions.yml")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("expected staged config: %v", err)
	}

	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/esb-lambda-base:staging") {
		t.Fatalf("expected base image build")
	}
	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/hello:staging") {
		t.Fatalf("expected function image build")
	}
	if !hasDockerPushTag(dockerRunner.calls, "localhost:5010/esb-lambda-base:staging") {
		t.Fatalf("expected base image push")
	}
	if !hasDockerPushTag(dockerRunner.calls, "localhost:5010/hello:staging") {
		t.Fatalf("expected function image push")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, "com.esb.managed=true") {
		t.Fatalf("expected managed label on build")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, "com.esb.project=demo-staging") {
		t.Fatalf("expected project label on build")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, "com.esb.env=staging") {
		t.Fatalf("expected env label on build")
	}

	if !hasComposeUpRegistry(composeRunner.calls) {
		t.Fatalf("expected registry compose up")
	}
}

func TestGoBuilderBuildFirecrackerBuildsServiceImages(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	cfg := config.GeneratorConfig{
		App: config.AppConfig{Name: "demo"},
		Environments: config.Environments{
			{Name: "prod", Mode: "firecracker"},
		},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
	}
	if err := config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg); err != nil {
		t.Fatalf("write generator.yml: %v", err)
	}

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot,
		"docker-compose.yml",
		"docker-compose.worker.yml",
		"docker-compose.registry.yml",
		"docker-compose.fc.yml",
	)
	// Create mock service directories and root files required by staging logic
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "pyproject.toml"), "[project]\n")
	writeTestFile(t, filepath.Join(repoRoot, "cli", "internal", "generator", "assets", "Dockerfile.lambda-base"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "runtime-node", "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "runtime-node", "Dockerfile.firecracker"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "agent", "Dockerfile"), "FROM scratch\n")

	generate := func(cfg config.GeneratorConfig, _ GenerateOptions) ([]FunctionSpec, error) {
		outputDir := cfg.Paths.OutputDir
		writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}")
		writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []FunctionSpec{{Name: "hello"}}, nil
	}

	dockerRunner := &recordRunner{}
	composeRunner := &recordRunner{}
	setupRootCA(t)
	builder := &GoBuilder{
		Runner:        dockerRunner,
		ComposeRunner: composeRunner,
		BuildCompose: func(_ context.Context, _ compose.CommandRunner, _ compose.BuildOptions) error {
			return nil
		},
		Generate:     generate,
		FindRepoRoot: func(string) (string, error) { return repoRoot, nil },
	}

	request := app.BuildRequest{
		ProjectDir:   projectDir,
		TemplatePath: templatePath,
		Env:          "prod",
	}
	if err := builder.Build(request); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/esb-runtime-node:prod") {
		t.Fatalf("expected runtime-node image build")
	}
	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/esb-agent:prod") {
		t.Fatalf("expected agent image build")
	}
}

type recordRunner struct {
	calls []commandCall
	err   error
}

type commandCall struct {
	dir  string
	name string
	args []string
}

func (r *recordRunner) Run(_ context.Context, dir, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{
		dir:  dir,
		name: name,
		args: append([]string{}, args...),
	})
	return r.err
}

func (r *recordRunner) RunQuiet(_ context.Context, dir, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{
		dir:  dir,
		name: name,
		args: append([]string{}, args...),
	})
	return r.err
}

func (r *recordRunner) RunOutput(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, commandCall{
		dir:  dir,
		name: name,
		args: append([]string{}, args...),
	})
	return nil, r.err
}

func hasDockerBuildTag(calls []commandCall, tag string) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 3 {
			continue
		}
		if call.args[0] != "build" {
			continue
		}
		for i := 0; i+1 < len(call.args); i++ {
			if call.args[i] == "-t" && call.args[i+1] == tag {
				return true
			}
		}
	}
	return false
}

func hasDockerPushTag(calls []commandCall, tag string) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 2 {
			continue
		}
		if call.args[0] == "push" && call.args[1] == tag {
			return true
		}
	}
	return false
}

func hasDockerBuildLabel(calls []commandCall, label string) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 3 {
			continue
		}
		if call.args[0] != "build" {
			continue
		}
		for i := 0; i+1 < len(call.args); i++ {
			if call.args[i] == "--label" && call.args[i+1] == label {
				return true
			}
		}
	}
	return false
}

func hasComposeUpRegistry(calls []commandCall) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 4 {
			continue
		}
		if call.args[0] != "compose" {
			continue
		}
		for i := 0; i < len(call.args); i++ {
			if call.args[i] == "up" && i+2 < len(call.args) && call.args[i+1] == "-d" && call.args[i+2] == "registry" {
				return true
			}
		}
	}
	return false
}

func setupRootCA(t *testing.T) string {
	t.Helper()

	caDir := t.TempDir()
	caPath := filepath.Join(caDir, esbRootCACertName)
	writeTestFile(t, caPath, "root-CA")
	t.Setenv("ESB_CA_CERT_PATH", caPath)
	return caPath
}

func writeComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeTestFile(t, filepath.Join(root, name), "version: '3'\n")
	}
}
