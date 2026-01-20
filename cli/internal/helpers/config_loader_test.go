// Where: cli/internal/helpers/config_loader_test.go
// What: Tests for global config loader helpers.
// Why: Ensure defaults / normalization despite missing files.
package helpers

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
)

func TestDefaultGlobalConfigLoader_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixConfigHome), dir)

	loader := DefaultGlobalConfigLoader()
	path, cfg, err := loader()
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if path == "" {
		t.Fatalf("expected path, got empty")
	}
	if cfg.Version == 0 {
		t.Fatalf("expected version initialized, got %d", cfg.Version)
	}
	if cfg.Projects == nil {
		t.Fatalf("expected Projects map, got nil")
	}
}

func TestDefaultGlobalConfigLoader_Normalize(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envutil.HostEnvKey(constants.HostSuffixConfigHome), dir)
	cfg := config.GlobalConfig{
		Version: 0,
	}
	path, err := config.GlobalConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	cfg.Projects = nil
	if err := config.SaveGlobalConfig(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loader := DefaultGlobalConfigLoader()
	resolvedPath, normalized, err := loader()
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if resolvedPath != path {
		t.Fatalf("expected %s, got %s", path, resolvedPath)
	}
	if normalized.Version == 0 {
		t.Fatalf("expected normalized version")
	}
	if normalized.Projects == nil {
		t.Fatalf("expected Projects map")
	}
}
