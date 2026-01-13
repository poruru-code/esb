// Where: cli/internal/app/up_test.go
// What: Tests for up command wiring.
// Why: Ensure up command invokes the upper with resolved context.
package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
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

type fakeProvisioner struct {
	calls    int
	requests []ProvisionRequest
	err      error
}

func (f *fakeProvisioner) Provision(request ProvisionRequest) error {
	f.calls++
	f.requests = append(f.requests, request)
	return f.err
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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

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

func TestRunUpWithEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

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
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.yml"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write compose fixture: %v", err)
	}
	if err := writeGeneratorFixture(repoRoot, "staging"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, repoRoot, "demo")

	stagingDir := filepath.Join(repoRoot, "services", "gateway", ".esb-staging", "staging", "config")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}

	t.Setenv("ESB_ENV", "default")
	t.Setenv("ESB_PROJECT_NAME", "")
	t.Setenv("ESB_IMAGE_TAG", "")
	t.Setenv("ESB_CONFIG_DIR", "")
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:         &out,
		ProjectDir:  repoRoot,
		Upper:       upper,
		Provisioner: provisioner,
		RepoResolver: func(_ string) (string, error) {
			return repoRoot, nil
		},
	}

	exitCode := Run([]string{"--env", "staging", "up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := os.Getenv("ESB_ENV"); got != "staging" {
		t.Fatalf("unexpected ESB_ENV: %s", got)
	}
	if got := os.Getenv("ESB_PROJECT_NAME"); got != expectedComposeProject("demo", "staging") {
		t.Fatalf("unexpected ESB_PROJECT_NAME: %s", got)
	}
	if got := os.Getenv("ESB_IMAGE_TAG"); got != "staging" {
		t.Fatalf("unexpected ESB_IMAGE_TAG: %s", got)
	}
	expectedConfigDir := filepath.ToSlash(filepath.Join("services", "gateway", ".esb-staging", "staging", "config"))
	if got := os.Getenv("ESB_CONFIG_DIR"); got != expectedConfigDir {
		t.Fatalf("unexpected ESB_CONFIG_DIR: %s", got)
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
			OutputDir:    ".esb/",
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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

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

	t.Setenv("ESB_ENV", "custom")
	t.Setenv("ESB_PROJECT_NAME", "custom-project")
	t.Setenv("ESB_IMAGE_TAG", "custom-tag")
	t.Setenv("ESB_CONFIG_DIR", "custom/config")
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

	exitCode := Run([]string{"--env", "default", "up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := os.Getenv("ESB_ENV"); got != "default" {
		t.Fatalf("unexpected ESB_ENV: %s", got)
	}
	if got := os.Getenv("ESB_PROJECT_NAME"); got != "custom-project" {
		t.Fatalf("unexpected ESB_PROJECT_NAME: %s", got)
	}
	if got := os.Getenv("ESB_IMAGE_TAG"); got != "custom-tag" {
		t.Fatalf("unexpected ESB_IMAGE_TAG: %s", got)
	}
	if got := os.Getenv("ESB_CONFIG_DIR"); got != "custom/config" {
		t.Fatalf("unexpected ESB_CONFIG_DIR: %s", got)
	}
}

func TestRunUpMissingUpper(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	t.Setenv("ESB_MODE", "")

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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper}

	exitCode := Run([]string{"up"}, deps)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for missing provisioner")
	}
}

type fakeUpBuilder struct {
	requests []BuildRequest
	err      error
}

func (f *fakeUpBuilder) Build(request BuildRequest) error {
	f.requests = append(f.requests, request)
	return f.err
}

func TestRunUpWithBuildRunsBuilder(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	builder := &fakeUpBuilder{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Builder: builder, Provisioner: provisioner}

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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	waiter := &fakeWaiter{}
	var out bytes.Buffer
	deps := Dependencies{
		Out:         &out,
		ProjectDir:  projectDir,
		Upper:       upper,
		Provisioner: provisioner,
		Waiter:      waiter,
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
	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	waiter := &fakeWaiter{err: errors.New("boom")}
	var out bytes.Buffer
	deps := Dependencies{
		Out:         &out,
		ProjectDir:  projectDir,
		Upper:       upper,
		Provisioner: provisioner,
		Waiter:      waiter,
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

	t.Setenv("ESB_MODE", "")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

	exitCode := Run([]string{"up"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := os.Getenv("ESB_MODE"); got != "containerd" {
		t.Fatalf("unexpected ESB_MODE: %s", got)
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
	t.Setenv("ESB_MODE", "")
	setupProjectConfig(t, projectDir, "demo")
	t.Setenv("ESB_ENV", "staging")

	upper := &fakeUpper{}
	provisioner := &fakeProvisioner{}
	var out bytes.Buffer
	deps := Dependencies{Out: &out, ProjectDir: projectDir, Upper: upper, Provisioner: provisioner}

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
