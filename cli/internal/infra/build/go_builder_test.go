// Where: cli/internal/infra/build/go_builder_test.go
// What: Tests for GoBuilder build workflow.
// Why: Ensure Go-based build wiring matches expected config/output behavior.
package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	templategen "github.com/poruru/edge-serverless-box/cli/internal/infra/templategen"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func TestGoBuilderBuildGeneratesAndBuilds(t *testing.T) {
	t.Setenv("ENV_PREFIX", meta.EnvPrefix)
	t.Setenv("ESB_REGISTRY_WAIT", "0")
	registryKey, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(registryKey, "registry:5010")
	t.Setenv(constants.EnvNetworkExternal, "demo-external")
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot,
		"docker-compose.containerd.yml",
	)
	setWorkingDir(t, repoRoot)
	// Create mock service directories and root files required by staging logic
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "gateway"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "pyproject.toml"), "[project]\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, "runtime", "python", "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "runtime", "python", "docker", "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "gateway", "Dockerfile.containerd"), "FROM scratch\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "provisioner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "runtime-node"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "services", "agent", "Dockerfile.containerd"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "provisioner", "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "runtime-node", "Dockerfile.containerd"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "docker-bake.hcl"), "# bake stub\n")

	var gotCfg config.GeneratorConfig
	var gotOpts templategen.GenerateOptions
	generate := func(cfg config.GeneratorConfig, opts templategen.GenerateOptions) ([]template.FunctionSpec, error) {
		gotCfg = cfg
		gotOpts = opts

		outputDir := cfg.Paths.OutputDir
		writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}")
		writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []")
		writeTestFile(t, filepath.Join(outputDir, "config", "resources.yml"), "resources: {}")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []template.FunctionSpec{{Name: "hello", ImageName: "hello"}}, nil
	}

	builderName := meta.Slug + "-buildx"
	dockerRunner := &recordRunner{
		outputs: map[string][]byte{
			"git rev-parse --show-toplevel":                  []byte(repoRoot),
			"git rev-parse --git-dir":                        []byte(".git"),
			"git rev-parse --git-common-dir":                 []byte(".git"),
			"docker buildx inspect --builder " + builderName: []byte("Driver: docker-container\n"),
			"docker inspect -f {{.HostConfig.NetworkMode}} buildx_buildkit_" + builderName + "0": []byte("host"),
		},
	}
	composeRunner := &recordRunner{}
	portDiscoverer := &mockPortDiscoverer{
		ports: map[string]int{
			constants.EnvPortRegistry: 5010,
		},
	}

	builder := &GoBuilder{
		Runner:         dockerRunner,
		ComposeRunner:  composeRunner,
		PortDiscoverer: portDiscoverer,
		Generate:       generate,
		FindRepoRoot:   func(string) (string, error) { return repoRoot, nil },
	}

	modeKey, err := envutil.HostEnvKey(constants.HostSuffixMode)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(modeKey, "")
	t.Setenv(constants.EnvConfigDir, "")
	t.Setenv(constants.EnvProjectName, "")

	setupRootCA(t)
	request := BuildRequest{
		ProjectDir:   projectDir,
		ProjectName:  "demo-staging",
		TemplatePath: templatePath,
		Env:          "staging",
		Mode:         "containerd",
		BuildImages:  true,
		ImageRuntimes: map[string]string{
			"lambda-image": "java21",
		},
		Tag: "v1.2.3",
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
	if gotOpts.BuildRegistry != "127.0.0.1:5010/" {
		t.Fatalf("unexpected build registry: %s", gotOpts.BuildRegistry)
	}
	if gotOpts.RuntimeRegistry != "registry:5010/" {
		t.Fatalf("unexpected runtime registry: %s", gotOpts.RuntimeRegistry)
	}
	if gotOpts.Tag != "v1.2.3" {
		t.Fatalf("unexpected tag: %s", gotOpts.Tag)
	}
	if gotOpts.ImageRuntimes["lambda-image"] != "java21" {
		t.Fatalf("unexpected image runtimes: %#v", gotOpts.ImageRuntimes)
	}
	if gotCfg.Parameters["S3_ENDPOINT_HOST"] != "s3-storage" {
		t.Fatalf("missing S3_ENDPOINT_HOST parameter")
	}
	if gotCfg.Parameters["DYNAMODB_ENDPOINT_HOST"] != "database" {
		t.Fatalf("missing DYNAMODB_ENDPOINT_HOST parameter")
	}

	expectedConfigDir, err := staging.ConfigDir(templatePath, "demo-staging", "staging")
	if err != nil {
		t.Fatalf("resolve staging config dir: %v", err)
	}
	expectedConfigDir = filepath.ToSlash(expectedConfigDir)
	if got := os.Getenv(constants.EnvConfigDir); got != expectedConfigDir {
		t.Fatalf("unexpected %s: %s", constants.EnvConfigDir, got)
	}
	if got := os.Getenv(constants.EnvProjectName); got != "demo-staging" {
		t.Fatalf("unexpected %s: %s", constants.EnvProjectName, got)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixMode); err != nil {
		t.Fatal(err)
	} else if got != "containerd" {
		t.Fatalf("unexpected %s: %s", constants.HostSuffixMode, got)
	}

	stagingDir, err := staging.ConfigDir(templatePath, "demo-staging", "staging")
	if err != nil {
		t.Fatalf("resolve staging config dir: %v", err)
	}
	staged := filepath.Join(stagingDir, "functions.yml")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("expected staged config: %v", err)
	}

	if !hasDockerBakeGroup(dockerRunner.calls, "esb-base") {
		t.Fatalf("expected bake for base images")
	}
	if !hasDockerBakeGroup(dockerRunner.calls, "esb-functions") {
		t.Fatalf("expected bake for function images")
	}
	// Control plane images are now built separately via `esb build-infra` or docker compose
	// and are no longer part of the deploy workflow.
	if !hasDockerBakeProvenance(dockerRunner.calls, "mode=max") {
		t.Fatalf("expected provenance enabled by default")
	}
	if !hasBakeFileContaining(dockerRunner.bakeFiles, "type=registry") {
		t.Fatalf("expected registry output in bake file")
	}
	if !hasBakeFileContaining(dockerRunner.bakeFiles, "output = [\"type=docker\"]") {
		t.Fatalf("expected docker output in bake file")
	}
	// Control plane images are no longer built by the deploy workflow
	// They are built separately via `esb build-infra` or docker compose
	if !hasBakeFileContaining(dockerRunner.bakeFiles, meta.LabelPrefix+".managed") {
		t.Fatalf("expected managed label in bake file")
	}
	if !hasBakeFileContaining(dockerRunner.bakeFiles, meta.LabelPrefix+".project") {
		t.Fatalf("expected project label in bake file")
	}
	if !hasBakeFileContaining(dockerRunner.bakeFiles, meta.LabelPrefix+".env") {
		t.Fatalf("expected env label in bake file")
	}

	// Build cache is now managed separately and is not part of the deploy workflow
	// Cache configuration is handled by the docker compose build process

	// Registry is now started separately via `esb up` or docker compose
	// and is no longer part of the deploy workflow
}

func TestGoBuilderBuildRenderOnlySkipsImageBuilds(t *testing.T) {
	t.Setenv("ENV_PREFIX", meta.EnvPrefix)
	t.Setenv("ESB_REGISTRY_WAIT", "0")
	registryKey, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(registryKey, "registry:5010")
	t.Setenv(constants.EnvNetworkExternal, "demo-external")

	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	repoRoot := projectDir
	writeComposeFiles(t, repoRoot, "docker-compose.containerd.yml")
	setWorkingDir(t, repoRoot)
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "services", "gateway"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "pyproject.toml"), "[project]\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, "runtime", "python", "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "runtime", "python", "docker", "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "services", "gateway", "Dockerfile.containerd"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(repoRoot, "docker-bake.hcl"), "# bake stub\n")

	var gotOpts templategen.GenerateOptions
	generate := func(cfg config.GeneratorConfig, opts templategen.GenerateOptions) ([]template.FunctionSpec, error) {
		gotOpts = opts
		outputDir := cfg.Paths.OutputDir
		writeTestFile(t, filepath.Join(outputDir, "config", "functions.yml"), "functions: {}")
		writeTestFile(t, filepath.Join(outputDir, "config", "routing.yml"), "routes: []")
		writeTestFile(t, filepath.Join(outputDir, "config", "resources.yml"), "resources: {}")
		writeTestFile(t, filepath.Join(outputDir, "functions", "hello", "Dockerfile"), "FROM scratch\n")
		return []template.FunctionSpec{{Name: "hello", ImageName: "hello"}}, nil
	}

	dockerRunner := &recordRunner{
		outputs: map[string][]byte{
			"git rev-parse --show-toplevel":  []byte(repoRoot),
			"git rev-parse --git-dir":        []byte(".git"),
			"git rev-parse --git-common-dir": []byte(".git"),
		},
	}
	composeRunner := &recordRunner{}
	portDiscoverer := &mockPortDiscoverer{
		err: context.Canceled,
	}

	builder := &GoBuilder{
		Runner:         dockerRunner,
		ComposeRunner:  composeRunner,
		PortDiscoverer: portDiscoverer,
		Generate:       generate,
		FindRepoRoot:   func(string) (string, error) { return repoRoot, nil },
	}

	modeKey, err := envutil.HostEnvKey(constants.HostSuffixMode)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(modeKey, "")
	t.Setenv(constants.EnvConfigDir, "")
	t.Setenv(constants.EnvProjectName, "")

	request := BuildRequest{
		ProjectDir:   projectDir,
		ProjectName:  "demo-staging",
		TemplatePath: templatePath,
		Env:          "staging",
		Mode:         "containerd",
		BuildImages:  false,
		Tag:          "v1.2.3",
	}
	if err := builder.Build(request); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hasDockerBakeGroup(dockerRunner.calls, "esb-base") {
		t.Fatalf("render-only build must not run base image bake")
	}
	if hasDockerBakeGroup(dockerRunner.calls, "esb-functions") {
		t.Fatalf("render-only build must not run function image bake")
	}
	if hasDockerCommand(dockerRunner.calls, "buildx", "inspect") {
		t.Fatalf("render-only build must not run buildx inspect")
	}
	if gotOpts.BuildRegistry != "registry:5010/" {
		t.Fatalf("unexpected render-only build registry: %s", gotOpts.BuildRegistry)
	}
	if gotOpts.RuntimeRegistry != "registry:5010/" {
		t.Fatalf("unexpected render-only runtime registry: %s", gotOpts.RuntimeRegistry)
	}
}

func setWorkingDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd %s: %v", wd, err)
		}
	})
}

func TestSortedAnyKeys(t *testing.T) {
	keys := sortedAnyKeys(map[string]any{
		"b": 2,
		"a": 1,
		"c": 3,
	})
	want := []string{"a", "b", "c"}
	if len(keys) != len(want) {
		t.Fatalf("unexpected key count: %v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("unexpected order: %v", keys)
		}
	}
}

type recordRunner struct {
	calls     []commandCall
	err       error
	outputs   map[string][]byte
	bakeFiles map[string]string
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
	r.captureBakeFiles(dir, name, args)
	return r.err
}

func (r *recordRunner) RunQuiet(_ context.Context, dir, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{
		dir:  dir,
		name: name,
		args: append([]string{}, args...),
	})
	r.captureBakeFiles(dir, name, args)
	return r.err
}

func (r *recordRunner) RunOutput(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, commandCall{
		dir:  dir,
		name: name,
		args: append([]string{}, args...),
	})
	r.captureBakeFiles(dir, name, args)
	if r.err != nil {
		return nil, r.err
	}
	if r.outputs != nil {
		key := name + " " + strings.Join(args, " ")
		if output, ok := r.outputs[key]; ok {
			return output, nil
		}
	}
	return nil, nil
}

func (r *recordRunner) captureBakeFiles(dir, name string, args []string) {
	if name != "docker" || len(args) < 2 {
		return
	}
	if args[0] != "buildx" || args[1] != "bake" {
		return
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-f" {
			continue
		}
		path := args[i+1]
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if r.bakeFiles == nil {
			r.bakeFiles = make(map[string]string)
		}
		r.bakeFiles[path] = string(data)
	}
}

func hasDockerBakeGroup(calls []commandCall, group string) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 3 {
			continue
		}
		if call.args[0] != "buildx" || call.args[1] != "bake" {
			continue
		}
		for _, arg := range call.args {
			if arg == group {
				return true
			}
		}
	}
	return false
}

func hasDockerCommand(calls []commandCall, argsPrefix ...string) bool {
	for _, call := range calls {
		if call.name != "docker" {
			continue
		}
		if len(call.args) < len(argsPrefix) {
			continue
		}
		matched := true
		for i := range argsPrefix {
			if call.args[i] != argsPrefix[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func hasBakeFileContaining(files map[string]string, needle string) bool {
	if strings.TrimSpace(needle) == "" {
		return false
	}
	for _, content := range files {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func hasDockerBakeProvenance(calls []commandCall, mode string) bool {
	for _, call := range calls {
		if call.name != "docker" || len(call.args) < 2 {
			continue
		}
		if call.args[0] != "buildx" || call.args[1] != "bake" {
			continue
		}
		for _, arg := range call.args {
			if arg == "--provenance="+mode {
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
	caKey, err := envutil.HostEnvKey(constants.HostSuffixCACertPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(caKey, caPath)
	return caPath
}

func writeComposeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeTestFile(t, filepath.Join(root, name), "version: '3'\n")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
