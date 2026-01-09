// Where: cli/internal/config/global_test.go
// What: Tests for global config helpers.
// Why: Ensure global config round-trips correctly.
package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestGlobalConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := GlobalConfig{
		Version:       1,
		ActiveProject: "my-app",
		ActiveEnvironments: map[string]string{
			"my-app": "staging",
		},
		Projects: map[string]ProjectEntry{
			"my-app": {
				Path:     "/path/to/app",
				LastUsed: "2026-01-08T23:45:00+09:00",
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
