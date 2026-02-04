// Where: cli/internal/infra/config/global_test.go
// What: Tests for global config env.
// Why: Ensure global config round-trips correctly.
package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func TestGlobalConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := GlobalConfig{
		Version: 1,
		RecentTemplates: []string{
			"/tmp/template.yaml",
			"/work/template.yml",
		},
		Projects: map[string]ProjectEntry{
			"my-app": {
				Path:     "/path/to/app",
				LastUsed: "2026-01-08T23:45:00+09:00",
			},
		},
		BuildDefaults: map[string]BuildDefaults{
			"/tmp/template.yaml": {
				Env:       "staging",
				Mode:      "docker",
				OutputDir: meta.OutputDir,
				Params: map[string]string{
					"ParamA": "value-a",
				},
			},
		},
	}

	if err := SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	loaded, err := LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}

	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config mismatch: expected %#v, got %#v", cfg, loaded)
	}
}

func TestProjectConfigPathUsesProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	got, err := ProjectConfigPath(projectRoot)
	if err != nil {
		t.Fatalf("project config path: %v", err)
	}
	want := filepath.Join(projectRoot, meta.HomeDir, "config.yaml")
	if got != want {
		t.Fatalf("unexpected config path: %s", got)
	}
}

func TestEnsureProjectConfigCreatesDefault(t *testing.T) {
	projectRoot := t.TempDir()
	path := filepath.Join(projectRoot, meta.HomeDir, "config.yaml")

	if err := EnsureProjectConfig(projectRoot); err != nil {
		t.Fatalf("ensure project config: %v", err)
	}

	loaded, err := LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}

	expected := DefaultGlobalConfig()
	if loaded.Version != expected.Version {
		t.Fatalf("unexpected version: %d", loaded.Version)
	}
	if len(loaded.Projects) != 0 {
		t.Fatalf("expected empty projects, got %#v", loaded.Projects)
	}
}

func TestEnsureProjectConfigKeepsExisting(t *testing.T) {
	projectRoot := t.TempDir()
	path := filepath.Join(projectRoot, meta.HomeDir, "config.yaml")

	cfg := GlobalConfig{
		Version: 2,
		Projects: map[string]ProjectEntry{
			"demo": {Path: "/tmp/demo"},
		},
	}
	if err := SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save project config: %v", err)
	}

	if err := EnsureProjectConfig(projectRoot); err != nil {
		t.Fatalf("ensure project config: %v", err)
	}

	loaded, err := LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config mismatch: expected %#v, got %#v", cfg, loaded)
	}
}
