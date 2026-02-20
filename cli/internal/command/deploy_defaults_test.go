// Where: cli/internal/command/deploy_defaults_test.go
// What: Unit tests for deploy default helper functions.
// Why: Keep fallback and map-clone behavior deterministic and safe.
package command

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/poruru-code/esb/cli/internal/constants"
	"github.com/poruru-code/esb/cli/internal/infra/config"
)

func TestResolveTemplateFallbackUsesPrevious(t *testing.T) {
	path := writeDefaultsTemplate(t, "previous.yaml")

	got, err := resolveTemplateFallback(path, nil)
	if err != nil {
		t.Fatalf("resolve template fallback: %v", err)
	}
	if got != path {
		t.Fatalf("expected previous path %q, got %q", path, got)
	}
}

func TestResolveTemplateFallbackUsesCandidate(t *testing.T) {
	path := writeDefaultsTemplate(t, "candidate.yaml")

	got, err := resolveTemplateFallback("", []string{path})
	if err != nil {
		t.Fatalf("resolve template fallback: %v", err)
	}
	if got != path {
		t.Fatalf("expected candidate path %q, got %q", path, got)
	}
}

func TestResolveTemplateFallbackUsesCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	got, err := resolveTemplateFallback("", nil)
	if err != nil {
		t.Fatalf("resolve template fallback: %v", err)
	}
	if got != templatePath {
		t.Fatalf("expected template path %q, got %q", templatePath, got)
	}
}

func TestCloneParamsCopiesMap(t *testing.T) {
	src := map[string]string{"A": "1", "B": "2"}
	cloned := cloneParams(src)
	if len(cloned) != len(src) {
		t.Fatalf("expected len %d, got %d", len(src), len(cloned))
	}
	cloned["A"] = "changed"
	if src["A"] != "1" {
		t.Fatalf("expected src to remain unchanged, got %q", src["A"])
	}
	if cloneParams(nil) != nil {
		t.Fatal("expected nil clone for nil source")
	}
}

func TestSaveAndLoadDeployDefaults(t *testing.T) {
	projectRoot := t.TempDir()
	templatePath := writeDefaultsTemplate(t, "template.yaml")
	templateInput := deployTemplateInput{
		TemplatePath:  templatePath,
		OutputDir:     ".esb/out/template",
		Parameters:    map[string]string{"Key": "Value"},
		ImageSources:  map[string]string{"lambda-image": "public.ecr.aws/example/repo:latest"},
		ImageRuntimes: map[string]string{"lambda-image": "java21"},
	}
	inputs := deployInputs{
		Env:  "dev",
		Mode: "docker",
	}

	if err := saveDeployDefaults(projectRoot, templateInput, inputs); err != nil {
		t.Fatalf("save deploy defaults: %v", err)
	}
	loaded := loadDeployDefaults(projectRoot, templatePath)
	if loaded.Env != "dev" || loaded.Mode != "docker" {
		t.Fatalf("unexpected loaded env/mode: %#v", loaded)
	}
	if loaded.OutputDir != templateInput.OutputDir {
		t.Fatalf("unexpected output dir: %q", loaded.OutputDir)
	}
	if loaded.Params["Key"] != "Value" {
		t.Fatalf("unexpected params: %#v", loaded.Params)
	}
	if loaded.ImageSources["lambda-image"] != "public.ecr.aws/example/repo:latest" {
		t.Fatalf("unexpected image sources: %#v", loaded.ImageSources)
	}
	if loaded.ImageRuntimes["lambda-image"] != "java21" {
		t.Fatalf("unexpected image runtimes: %#v", loaded.ImageRuntimes)
	}

	cfgPath, err := config.ProjectConfigPath(projectRoot)
	if err != nil {
		t.Fatalf("project config path: %v", err)
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if len(cfg.RecentTemplates) == 0 || cfg.RecentTemplates[0] != templatePath {
		t.Fatalf("unexpected template history: %#v", cfg.RecentTemplates)
	}
}

func TestSaveDeployDefaultsSkipsEmptyTemplatePath(t *testing.T) {
	if err := saveDeployDefaults(t.TempDir(), deployTemplateInput{}, deployInputs{}); err != nil {
		t.Fatalf("expected nil error for empty template path, got %v", err)
	}
}

func TestResolveBrandTagUsesHostEnvTag(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_"+constants.HostSuffixTag, "v1.2.3")

	got := resolveBrandTag()
	if got != "v1.2.3" {
		t.Fatalf("expected v1.2.3, got %q", got)
	}
}

func TestResolveBrandTagDefaultsToLatest(t *testing.T) {
	t.Setenv("ENV_PREFIX", "")
	if got := resolveBrandTag(); got != "latest" {
		t.Fatalf("expected latest, got %q", got)
	}
}

func TestLoadTemplateHistoryFiltersDedupesAndLimits(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte(""), 0o600); err != nil {
		t.Fatalf("write repo marker: %v", err)
	}
	templatePaths := make([]string, 0, templateHistoryLimit+2)
	for i := 0; i < templateHistoryLimit+2; i++ {
		path := filepath.Join(repoRoot, fmt.Sprintf("template-%d.yaml", i))
		if err := os.WriteFile(path, []byte("Resources: {}"), 0o600); err != nil {
			t.Fatalf("write template %d: %v", i, err)
		}
		templatePaths = append(templatePaths, path)
	}
	cfgPath, err := config.ProjectConfigPath(repoRoot)
	if err != nil {
		t.Fatalf("project config path: %v", err)
	}
	cfg := config.DefaultGlobalConfig()
	cfg.RecentTemplates = append(
		[]string{"", templatePaths[0], templatePaths[1], templatePaths[0], filepath.Join(repoRoot, "missing.yaml")},
		templatePaths[2:]...,
	)
	if err := config.SaveGlobalConfig(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	workDir := filepath.Join(repoRoot, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	got := loadTemplateHistory()
	want := []string{
		templatePaths[0],
		templatePaths[1],
		templatePaths[2],
		templatePaths[3],
		templatePaths[4],
		templatePaths[5],
		templatePaths[6],
		templatePaths[7],
		templatePaths[8],
		templatePaths[9],
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected history: got=%v want=%v", got, want)
	}
}

func TestLoadTemplateHistoryReturnsNilOutsideRepo(t *testing.T) {
	wd := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := loadTemplateHistory(); got != nil {
		t.Fatalf("expected nil history outside repo, got %v", got)
	}
}

func writeDefaultsTemplate(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}
