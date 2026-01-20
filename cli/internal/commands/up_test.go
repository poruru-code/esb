// Where: cli/internal/commands/up_test.go
// What: Tests for up command wiring.
// Why: Ensure up command invokes the upper with resolved context.
package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type fakeUpper struct {
	calls    int
	requests []UpRequest
	err      error
}

func (f *fakeUpper) Up(request UpRequest) error {
	f.calls++
	f.requests = append(f.requests, request)
	return f.err
}

type fakeUpDowner struct {
	projects      []string
	removeVolumes []bool
	err           error
}

func (f *fakeUpDowner) Down(project string, removeVolumes bool) error {
	f.projects = append(f.projects, project)
	f.removeVolumes = append(f.removeVolumes, removeVolumes)
	return f.err
}

type fakeProvisioner struct {
	calls     int
	resources manifest.ResourcesSpec
	project   string
	err       error
}

func (f *fakeProvisioner) Apply(_ context.Context, resources manifest.ResourcesSpec, project string) error {
	f.calls++
	f.resources = resources
	f.project = project
	return f.err
}

type fakeParser struct {
	result generator.ParseResult
	err    error
}

func (f *fakeParser) Parse(string, map[string]string) (generator.ParseResult, error) {
	return f.result, f.err
}

type fakeWaiter struct {
	calls int
	err   error
}

func (f *fakeWaiter) Wait(state.Context) error {
	f.calls++
	return f.err
}

func TestRunUpCallsUpper(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixEnv), "default")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if upper.calls != 1 {
		t.Fatalf("expected upper called once, got %d", upper.calls)
	}
	if len(upper.requests) != 1 || upper.requests[0].Context.ComposeProject != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected project: %v", upper.requests)
	}
	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if upper.requests[0].Context.TemplatePath != expectedTemplate {
		t.Fatalf("unexpected template path: %s", upper.requests[0].Context.TemplatePath)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner called once, got %d", provisioner.calls)
	}
	if !upper.requests[0].Detach {
		t.Fatalf("expected detach to default to true")
	}
}

func TestRunUpResetCallsDownBuildUp(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	downer := &fakeUpDowner{}
	builder := &fakeUpBuilder{}
	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	waiter := &fakeWaiter{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Up: UpDeps{
			Downer:      downer,
			Builder:     builder,
			Upper:       upper,
			Provisioner: provisioner,
			Parser:      parser,
			Waiter:      waiter,
		},
	}

	exitCode := Run([]string{"up", "--reset", "--yes", "--wait"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(downer.projects) != 1 || downer.projects[0] != expectedComposeProject("demo", "default") {
		t.Fatalf("unexpected down project: %v", downer.projects)
	}
	if len(downer.removeVolumes) != 1 || !downer.removeVolumes[0] {
		t.Fatalf("expected removeVolumes true, got %v", downer.removeVolumes)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder called once, got %d", len(builder.requests))
	}
	if upper.calls != 1 {
		t.Fatalf("expected upper called once, got %d", upper.calls)
	}
	if len(upper.requests) != 1 || !upper.requests[0].Wait {
		t.Fatalf("expected wait to be true, got %v", upper.requests)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner called once, got %d", provisioner.calls)
	}
	if waiter.calls != 1 {
		t.Fatalf("expected waiter called once, got %d", waiter.calls)
	}
}

func TestRunUpResetRequiresYesInNonInteractiveMode(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	origIsTerminal := interaction.IsTerminal
	interaction.IsTerminal = func(_ *os.File) bool {
		return false
	}
	t.Cleanup(func() {
		interaction.IsTerminal = origIsTerminal
	})

	downer := &fakeUpDowner{}
	builder := &fakeUpBuilder{}
	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Up: UpDeps{
			Downer:      downer,
			Builder:     builder,
			Upper:       upper,
			Provisioner: provisioner,
			Parser:      parser,
		},
	}

	exitCode := Run([]string{"up", "--reset"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code without --yes")
	}
	if len(downer.projects) != 0 {
		t.Fatalf("expected downer not called, got %v", downer.projects)
	}
}

func TestRunUpOutputsLegacySuccess(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	t.Setenv("AUTH_USER", "user")
	t.Setenv("AUTH_PASS", "pass")
	t.Setenv("JWT_SECRET_KEY", "jwt")
	t.Setenv("X_API_KEY", "api")
	t.Setenv("RUSTFS_ACCESS_KEY", "access")
	t.Setenv("RUSTFS_SECRET_KEY", "secret")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	expected := "✓ Up complete\nNext:\n  esb logs <service>  # View logs\n  esb down            # Stop environment\n"
	if out.String() != expected {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunUpWithEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"--env", "staging", "up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(upper.requests) != 1 || upper.requests[0].Context.ComposeProject != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected context: %v", upper.requests)
	}
}

func TestRunUpAppliesEnvDefaults(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "compose"), 0o755); err != nil {
		t.Fatalf("create compose dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "compose", "base.yml"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write compose fixture: %v", err)
	}
	if err := writeGeneratorFixture(repoRoot, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, repoRoot, "demo")

	projectName := expectedComposeProject("demo", "staging")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	stagingDir := staging.ConfigDir(projectName, "staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}

	t.Setenv(envutil.HostEnvKey(constants.HostSuffixEnv), "default")
	t.Setenv(constants.EnvProjectName, "")
	t.Setenv(constants.EnvImageTag, "")
	t.Setenv(constants.EnvConfigDir, "")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: repoRoot,
		Up: UpDeps{
			Upper:       upper,
			Provisioner: provisioner,
			Parser:      parser,
		},
		RepoResolver: func(_ string) (string, error) {
			return repoRoot, nil
		},
	}

	exitCode := Run([]string{"--env", "staging", "up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := envutil.GetHostEnv(constants.HostSuffixEnv); got != "staging" {
		t.Fatalf("unexpected %s: %s", constants.HostSuffixEnv, got)
	}
	if got := os.Getenv(constants.EnvProjectName); got != projectName {
		t.Fatalf("unexpected %s: %s", constants.EnvProjectName, got)
	}
	if got := os.Getenv(constants.EnvImageTag); got != "docker" {
		t.Fatalf("unexpected %s: %s", constants.EnvImageTag, got)
	}
	expectedConfigDir := filepath.ToSlash(staging.ConfigDir(projectName, "staging"))
	if got := os.Getenv(constants.EnvConfigDir); got != expectedConfigDir {
		t.Fatalf("unexpected %s: %s", constants.EnvConfigDir, got)
	}
}

func TestRunUpAppliesGeneratorParameters(t *testing.T) {
	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cfg := config.GeneratorConfig{
		Environments: config.Environments{{Name: "default", Mode: "docker"}},
		Paths: config.PathsConfig{
			SamTemplate:  "template.yaml",
			OutputDir:    meta.OutputDir + "/",
			FunctionsYml: "custom/functions.yml",
			RoutingYml:   "custom/routing.yml",
		},
		Parameters: map[string]any{
			"RUSTFS_ACCESS_KEY": "test-access",
			"RETRY_COUNT":       3,
			"FEATURE_FLAG":      true,
			"COMPLEX": map[string]any{
				"nested": "value",
			},
		},
	}
	if err := config.SaveGeneratorConfig(filepath.Join(projectDir, "generator.yml"), cfg); err != nil {
		t.Fatalf("write generator config: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	t.Setenv("GATEWAY_FUNCTIONS_YML", "")
	t.Setenv("GATEWAY_ROUTING_YML", "")
	t.Setenv("RUSTFS_ACCESS_KEY", "")
	t.Setenv("RETRY_COUNT", "")
	t.Setenv("FEATURE_FLAG", "")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := os.Getenv("GATEWAY_FUNCTIONS_YML"); got != "custom/functions.yml" {
		t.Fatalf("unexpected GATEWAY_FUNCTIONS_YML: %s", got)
	}
	if got := os.Getenv("GATEWAY_ROUTING_YML"); got != "custom/routing.yml" {
		t.Fatalf("unexpected GATEWAY_ROUTING_YML: %s", got)
	}
	if got := os.Getenv("RUSTFS_ACCESS_KEY"); got != "test-access" {
		t.Fatalf("unexpected RUSTFS_ACCESS_KEY: %s", got)
	}
	if got := os.Getenv("RETRY_COUNT"); got != "3" {
		t.Fatalf("unexpected RETRY_COUNT: %s", got)
	}
	if got := os.Getenv("FEATURE_FLAG"); got != "true" {
		t.Fatalf("unexpected FEATURE_FLAG: %s", got)
	}
	if _, ok := os.LookupEnv("COMPLEX"); ok {
		t.Fatalf("unexpected COMPLEX env var to be set")
	}
}

func TestRunUpKeepsExplicitEnvOverrides(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	t.Setenv(envutil.HostEnvKey(constants.HostSuffixEnv), "custom")
	t.Setenv(constants.EnvProjectName, "custom-project")
	t.Setenv(constants.EnvImageTag, "custom-tag")
	t.Setenv(constants.EnvConfigDir, "custom/config")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"--env", "default", "up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := envutil.GetHostEnv(constants.HostSuffixEnv); got != "default" {
		t.Fatalf("unexpected %s: %s", constants.HostSuffixEnv, got)
	}
	if got := os.Getenv(constants.EnvProjectName); got != "custom-project" {
		t.Fatalf("unexpected %s: %s", constants.EnvProjectName, got)
	}
	if got := os.Getenv(constants.EnvImageTag); got != "custom-tag" {
		t.Fatalf("unexpected %s: %s", constants.EnvImageTag, got)
	}
	if got := os.Getenv(constants.EnvConfigDir); got != "custom/config" {
		t.Fatalf("unexpected %s: %s", constants.EnvConfigDir, got)
	}
}

func TestRunUpMissingUpper(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir}

	exitCode := Run([]string{"up"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing upper")
	}
}

func TestRunUpMissingProvisioner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing provisioner")
	}
}

type fakeUpBuilder struct {
	requests []generator.BuildRequest
	err      error
}

func (f *fakeUpBuilder) Build(request generator.BuildRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

func TestRunUpWithBuildRunsBuilder(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	builder := &fakeUpBuilder{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Builder: builder, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up", "--build"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected build called once, got %d", len(builder.requests))
	}
	req := builder.requests[0]
	expectedTemplate := filepath.Join(projectDir, "template.yaml")
	if req.TemplatePath != expectedTemplate {
		t.Fatalf("unexpected template path: %s", req.TemplatePath)
	}
	if req.Env != "default" {
		t.Fatalf("unexpected env: %s", req.Env)
	}
	if upper.calls != 1 {
		t.Fatalf("expected up called once, got %d", upper.calls)
	}
}

func TestRunUpWithBuildMissingBuilder(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up", "--build"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing builder")
	}
	if upper.calls != 0 {
		t.Fatalf("expected up not called when build fails")
	}
}

func TestRunUpWithWaitCallsWaiter(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	waiter := &fakeWaiter{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Up: UpDeps{
			Upper:       upper,
			Provisioner: provisioner,
			Parser:      parser,
			Waiter:      waiter,
		},
	}

	exitCode := Run([]string{"up", "--wait"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if waiter.calls != 1 {
		t.Fatalf("expected waiter called once, got %d", waiter.calls)
	}
}

func TestRunUpWithWaiterError(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	waiter := &fakeWaiter{err: errors.New("boom")}
	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Up: UpDeps{
			Upper:       upper,
			Provisioner: provisioner,
			Parser:      parser,
			Waiter:      waiter,
		},
	}

	exitCode := Run([]string{"up", "--wait"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code when waiter fails")
	}
	if waiter.calls != 1 {
		t.Fatalf("expected waiter called once, got %d", waiter.calls)
	}
}

func TestRunUpSetsModeFromGenerator(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixtureWithMode(projectDir, "default", "containerd"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := envutil.GetHostEnv(constants.HostSuffixMode); got != "containerd" {
		t.Fatalf("unexpected %s: %s", constants.HostSuffixMode, got)
	}
}

func TestRunUpUsesActiveEnvFromGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	envs := config.Environments{
		{Name: "default", Mode: "docker"},
		{Name: "staging", Mode: "containerd"},
	}
	if err := writeGeneratorFixtureWithEnvs(projectDir, envs, "demo"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixMode), "")
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixEnv), "staging")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	parser := &fakeParser{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Up: UpDeps{Upper: upper, Provisioner: provisioner, Parser: parser}}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(upper.requests) != 1 {
		t.Fatalf("expected upper called once")
	}
	if upper.requests[0].Context.Env != "staging" {
		t.Fatalf("unexpected env: %s", upper.requests[0].Context.Env)
	}
}
