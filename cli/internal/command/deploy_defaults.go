// Where: cli/internal/command/deploy_defaults.go
// What: Deploy default/history persistence helpers.
// Why: Separate state persistence from deploy input orchestration.
package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	domaintpl "github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

func loadTemplateHistory() []string {
	startDir, err := os.Getwd()
	if err != nil {
		return nil
	}
	repoRoot, err := config.ResolveRepoRoot(startDir)
	if err != nil {
		return nil
	}
	cfgPath, err := config.ProjectConfigPath(repoRoot)
	if err != nil {
		return nil
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		return nil
	}

	history := make([]string, 0, len(cfg.RecentTemplates))
	seen := map[string]struct{}{}
	for _, entry := range cfg.RecentTemplates {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		if _, err := os.Stat(trimmed); err != nil {
			continue
		}
		history = append(history, trimmed)
		seen[trimmed] = struct{}{}
		if len(history) >= templateHistoryLimit {
			break
		}
	}
	return history
}

func resolveTemplateFallback(previous string, candidates []string) (string, error) {
	if strings.TrimSpace(previous) != "" {
		return normalizeTemplatePath(previous)
	}
	if len(candidates) > 0 {
		return normalizeTemplatePath(candidates[0])
	}
	return normalizeTemplatePath(".")
}

func loadDeployDefaults(projectRoot, templatePath string) storedDeployDefaults {
	cfgPath, err := config.ProjectConfigPath(projectRoot)
	if err != nil || templatePath == "" {
		return storedDeployDefaults{}
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		return storedDeployDefaults{}
	}
	// Use BuildDefaults for now - can be separated later if needed.
	if cfg.BuildDefaults == nil {
		return storedDeployDefaults{}
	}
	entry, ok := cfg.BuildDefaults[templatePath]
	if !ok {
		return storedDeployDefaults{}
	}
	return storedDeployDefaults{
		Env:           entry.Env,
		Mode:          entry.Mode,
		OutputDir:     entry.OutputDir,
		Params:        cloneParams(entry.Params),
		ImageSources:  cloneStringMap(entry.ImageSources),
		ImageRuntimes: cloneStringMap(entry.ImageRuntimes),
	}
}

func saveDeployDefaults(projectRoot string, template deployTemplateInput, inputs deployInputs) error {
	if strings.TrimSpace(template.TemplatePath) == "" {
		return nil
	}
	cfgPath, err := config.ProjectConfigPath(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project config path: %w", err)
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		cfg = config.DefaultGlobalConfig()
	}
	if cfg.BuildDefaults == nil {
		cfg.BuildDefaults = map[string]config.BuildDefaults{}
	}
	cfg.BuildDefaults[template.TemplatePath] = config.BuildDefaults{
		Env:           inputs.Env,
		Mode:          inputs.Mode,
		OutputDir:     template.OutputDir,
		Params:        cloneParams(template.Parameters),
		ImageSources:  cloneStringMap(template.ImageSources),
		ImageRuntimes: cloneStringMap(template.ImageRuntimes),
	}
	cfg.RecentTemplates = domaintpl.UpdateHistory(cfg.RecentTemplates, template.TemplatePath, templateHistoryLimit)
	if err := config.SaveGlobalConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}
	return nil
}

func resolveBrandTag() string {
	// Use brand-prefixed environment variable (e.g., ESB_TAG).
	key, err := envutil.HostEnvKey(constants.HostSuffixTag)
	if err == nil {
		tag := os.Getenv(key)
		if tag != "" {
			return tag
		}
	}
	return "latest"
}

func cloneParams(src map[string]string) map[string]string {
	return cloneStringMap(src)
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
