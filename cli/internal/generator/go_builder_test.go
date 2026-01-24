// Where: cli/internal/generator/go_builder_test.go
// What: Tests for GoBuilder build workflow.
// Why: Ensure Go-based build wiring matches expected config/output behavior.
package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

func TestGoBuilderBuildGeneratesAndBuilds(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot,
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
		writeTestFile(t, filepath.Join(outputDir, "config", "resources.yml"), "resources: {}")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []FunctionSpec{{Name: "hello", ImageName: "hello"}}, nil
	}

	dockerRunner := &recordRunner{}
	composeRunner := &recordRunner{}
	portDiscoverer := &mockPortDiscoverer{
		ports: map[string]int{
			constants.EnvPortRegistry: 5010,
		},
	}
	var buildOpts compose.BuildOptions

	builder := &GoBuilder{
		Runner:         dockerRunner,
		ComposeRunner:  composeRunner,
		PortDiscoverer: portDiscoverer,
		BuildCompose: func(_ context.Context, _ compose.CommandRunner, opts compose.BuildOptions) error {
			buildOpts = opts
			return nil
		},
		Generate:     generate,
		FindRepoRoot: func(string) (string, error) { return repoRoot, nil },
	}

	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")
	t.Setenv(constants.EnvConfigDir, "")
	t.Setenv(constants.EnvProjectName, "")
	t.Setenv(constants.EnvImageTag, "")

	setupRootCA(t)
	request := BuildRequest{
		ProjectDir:   projectDir,
		ProjectName:  "demo-staging",
		TemplatePath: templatePath,
		Env:          "staging",
		Mode:         "containerd",
	}
	if err := builder.Build(request); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedOutput := filepath.Join(projectDir, meta.OutputDir, "staging")
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
	if gotCfg.Parameters["S3_ENDPOINT_HOST"] != "s3-storage" {
		t.Fatalf("missing S3_ENDPOINT_HOST parameter")
	}
	if gotCfg.Parameters["DYNAMODB_ENDPOINT_HOST"] != "database" {
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
	expectedServices := []string{"os-base", "python-base", "gateway", "agent", "provisioner"}
	if len(buildOpts.Services) != len(expectedServices) {
		t.Fatalf("unexpected compose services: %v", buildOpts.Services)
	}
	for i, service := range expectedServices {
		if buildOpts.Services[i] != service {
			t.Fatalf("unexpected compose services: %v", buildOpts.Services)
		}
	}

	expectedConfigDir := filepath.ToSlash(staging.ConfigDir("demo-staging", "staging"))
	if got := os.Getenv(constants.EnvConfigDir); got != expectedConfigDir {
		t.Fatalf("unexpected %s: %s", constants.EnvConfigDir, got)
	}
	if got := os.Getenv(constants.EnvProjectName); got != "demo-staging" {
		t.Fatalf("unexpected %s: %s", constants.EnvProjectName, got)
	}
	if got := os.Getenv(constants.EnvImageTag); got != "containerd" {
		t.Fatalf("unexpected %s: %s", constants.EnvImageTag, got)
	}
	if got := envutil.GetHostEnv(constants.HostSuffixMode); got != "containerd" {
		t.Fatalf("unexpected %s: %s", constants.HostSuffixMode, got)
	}

	staged := filepath.Join(staging.ConfigDir("demo-staging", "staging"), "functions.yml")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("expected staged config: %v", err)
	}

	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-lambda-base:staging") {
		t.Fatalf("expected base image build")
	}
	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-hello:staging") {
		t.Fatalf("expected function image build")
	}
	if !hasDockerPushTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-lambda-base:staging") {
		t.Fatalf("expected base image push")
	}
	if !hasDockerPushTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-hello:staging") {
		t.Fatalf("expected function image push")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, meta.LabelPrefix+".managed=true") {
		t.Fatalf("expected managed label on build")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, meta.LabelPrefix+".project=demo-staging") {
		t.Fatalf("expected project label on build")
	}
	if !hasDockerBuildLabel(dockerRunner.calls, meta.LabelPrefix+".env=staging") {
		t.Fatalf("expected env label on build")
	}

	if !hasComposeUpRegistry(composeRunner.calls) {
		t.Fatalf("expected registry compose up")
	}
}

func TestGoBuilderBuildFirecrackerBuildsServiceImages(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot,
		"docker-compose.fc.yml",
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
	writeTestFile(t, filepath.Join(repoRoot, "services", "runtime-node", "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "runtime-node", "Dockerfile.firecracker"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "agent", "Dockerfile"), "FROM scratch\n")

	generate := func(cfg config.GeneratorConfig, _ GenerateOptions) ([]FunctionSpec, error) {
		outputDir := cfg.Paths.OutputDir
		writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}")
		writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []")
		writeTestFile(t, filepath.Join(outputDir, "config", "resources.yml"), "resources: {}")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []FunctionSpec{{Name: "hello", ImageName: "hello"}}, nil
	}

	dockerRunner := &recordRunner{}
	composeRunner := &recordRunner{}
	portDiscoverer := &mockPortDiscoverer{
		ports: map[string]int{
			constants.EnvPortRegistry: 5010,
		},
	}
	setupRootCA(t)
	builder := &GoBuilder{
		Runner:         dockerRunner,
		ComposeRunner:  composeRunner,
		PortDiscoverer: portDiscoverer,
		BuildCompose: func(_ context.Context, _ compose.CommandRunner, _ compose.BuildOptions) error {
			return nil
		},
		Generate:     generate,
		FindRepoRoot: func(string) (string, error) { return repoRoot, nil },
	}

	request := BuildRequest{
		ProjectDir:   projectDir,
		ProjectName:  "demo-prod",
		TemplatePath: templatePath,
		Env:          "prod",
		Mode:         "firecracker",
	}
	if err := builder.Build(request); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-runtime-node:prod") {
		t.Fatalf("expected runtime-node image build")
	}
	if !hasDockerBuildTag(dockerRunner.calls, "localhost:5010/"+meta.ImagePrefix+"-agent:prod") {
		t.Fatalf("expected agent image build")
	}
}

type recordRunner struct {
	calls []commandCall
	err   error
}

type mockPortDiscoverer struct {
	ports map[string]int
	err   error
}

func (m *mockPortDiscoverer) Discover(_ context.Context, _, _, _ string) (map[string]int, error) {
	return m.ports, m.err
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
	caPath := filepath.Join(caDir, meta.RootCACertFilename)
	writeTestFile(t, caPath, "root-CA")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixCACertPath), caPath)
	return caPath
}

func writeComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeTestFile(t, filepath.Join(root, name), "version: '3'\n")
	}
}
