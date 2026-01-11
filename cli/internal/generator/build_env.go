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

func findRepoRoot(_ string) (string, error) {
	repo := os.Getenv("ESB_REPO")
	if repo == "" {
		return "", fmt.Errorf("ESB_REPO environment variable is not set")
	}

	info, err := os.Stat(repo)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("ESB_REPO directory does not exist: %s", repo)
	}

	return repo, nil
}
