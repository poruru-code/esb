// Where: cli/internal/config/generator_test.go
// What: Tests for generator.yml helpers.
// Why: Ensure configs round-trip correctly.
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGeneratorConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generator.yml")
	cfg := GeneratorConfig{
		App: AppConfig{
			Name: "my-app",
			Tag:  "default",
		},
		Environments: Environments{
			{Name: "default", Mode: "docker"},
			{Name: "staging", Mode: "containerd"},
		},
		Paths: PathsConfig{
			SamTemplate: "./template.yaml",
			OutputDir:   ".esb/",
		},
		Parameters: map[string]any{
			"TableName": "table",
			"Count":     2,
		},
	}

	if err := SaveGeneratorConfig(path, cfg); err != nil {
		t.Fatalf("save generator config: %v", err)
	}

	loaded, err := LoadGeneratorConfig(path)
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}

	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config mismatch: expected %#v, got %#v", cfg, loaded)
	}
}

func TestGeneratorConfigLoadList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generator.yml")
	content := `environments:
  - default
  - staging
paths:
  sam_template: ./template.yaml
  output_dir: .esb/
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write generator config: %v", err)
	}

	loaded, err := LoadGeneratorConfig(path)
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}

	if !loaded.Environments.Has("default") || !loaded.Environments.Has("staging") {
		t.Fatalf("unexpected environments: %#v", loaded.Environments)
	}
	if mode, ok := loaded.Environments.Mode("default"); ok && mode != "" {
		t.Fatalf("expected empty mode for list, got %q", mode)
	}
}

func TestGeneratorConfigLoadMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generator.yml")
	content := `environments:
  default: docker
  staging: containerd
paths:
  sam_template: ./template.yaml
  output_dir: .esb/
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write generator config: %v", err)
	}

	loaded, err := LoadGeneratorConfig(path)
	if err != nil {
		t.Fatalf("load generator config: %v", err)
	}

	if mode, ok := loaded.Environments.Mode("default"); !ok || mode != "docker" {
		t.Fatalf("unexpected mode for default: %q", mode)
	}
	if mode, ok := loaded.Environments.Mode("staging"); !ok || mode != "containerd" {
		t.Fatalf("unexpected mode for staging: %q", mode)
	}
}
