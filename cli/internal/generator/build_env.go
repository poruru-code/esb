// Where: cli/internal/generator/build_env.go
// What: Environment helpers for generator build flows.
// Why: Keep build-related env setup shared without Python builder dependency.
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func applyBuildEnv(env string) {
	configDir := filepath.Join("services", "gateway", ".esb-staging", env, "config")
	_ = os.Setenv("ESB_CONFIG_DIR", filepath.ToSlash(configDir))
	if os.Getenv("ESB_PROJECT_NAME") == "" {
		_ = os.Setenv("ESB_PROJECT_NAME", fmt.Sprintf("esb-%s", strings.ToLower(env)))
	}
	if os.Getenv("ESB_IMAGE_TAG") == "" {
		_ = os.Setenv("ESB_IMAGE_TAG", env)
	}
}

func applyModeFromConfig(cfg config.GeneratorConfig, env string) {
	if strings.TrimSpace(os.Getenv("ESB_MODE")) != "" {
		return
	}
	mode, ok := cfg.Environments.Mode(env)
	if !ok {
		return
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return
	}
	_ = os.Setenv("ESB_MODE", strings.ToLower(mode))
}

func findRepoRoot(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		if dir == "" || dir == string(filepath.Separator) {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, "tools", "cli", "main.py")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("repo root not found from %s", start)
}
