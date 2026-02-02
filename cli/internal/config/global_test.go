// Where: cli/internal/config/global_test.go
// What: Tests for global config helpers.
// Why: Ensure global config round-trips correctly.
package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

func setEnvPrefix(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_PREFIX", meta.EnvPrefix)
}

func TestGlobalConfigRoundTrip(t *testing.T) {
	setEnvPrefix(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := GlobalConfig{
		Version: 1,
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
				OutputDir: ".esb",
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

func TestGlobalConfigPathHonorsOverride(t *testing.T) {
	setEnvPrefix(t)
	baseDir := t.TempDir()
	overridePath := filepath.Join(baseDir, "custom", "config.yaml")
	t.Setenv("ESB_CONFIG_PATH", overridePath)

	got, err := GlobalConfigPath()
	if err != nil {
		t.Fatalf("global config path: %v", err)
	}
	if got != overridePath {
		t.Fatalf("unexpected config path: %s", got)
	}
}

func TestEnsureGlobalConfigCreatesDefault(t *testing.T) {
	setEnvPrefix(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("ESB_CONFIG_PATH", path)

	if err := EnsureGlobalConfig(); err != nil {
		t.Fatalf("ensure global config: %v", err)
	}

	loaded, err := LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}

	expected := DefaultGlobalConfig()
	if loaded.Version != expected.Version {
		t.Fatalf("unexpected version: %d", loaded.Version)
	}
	if len(loaded.Projects) != 0 {
		t.Fatalf("expected empty projects, got %#v", loaded.Projects)
	}
}

func TestEnsureGlobalConfigKeepsExisting(t *testing.T) {
	setEnvPrefix(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("ESB_CONFIG_PATH", path)

	cfg := GlobalConfig{
		Version: 2,
		Projects: map[string]ProjectEntry{
			"demo": {Path: "/tmp/demo"},
		},
	}
	if err := SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	if err := EnsureGlobalConfig(); err != nil {
		t.Fatalf("ensure global config: %v", err)
	}

	loaded, err := LoadGlobalConfig(path)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config mismatch: expected %#v, got %#v", cfg, loaded)
	}
}
