// Where: cli/internal/app/init.go
// What: Init command helpers.
// Why: Build and persist generator.yml for new projects.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

func runInit(templatePath string, envs []string, projectName string) (string, error) {
	cleaned := normalizeEnvs(envs)
	if len(cleaned) == 0 {
		cleaned = []string{"default"}
	}

	specs, err := parseEnvSpecs(cleaned)
	if err != nil {
		return "", err
	}

	cfg, generatorPath, err := buildGeneratorConfig(templatePath, specs, projectName)
	if err != nil {
		return "", err
	}

	if err := config.SaveGeneratorConfig(generatorPath, cfg); err != nil {
		return "", err
	}

	return generatorPath, nil
}

func buildGeneratorConfig(templatePath string, envs config.Environments, projectName string) (config.GeneratorConfig, string, error) {
	if templatePath == "" {
		return config.GeneratorConfig{}, "", fmt.Errorf("template path is required")
	}
	if len(envs) == 0 {
		return config.GeneratorConfig{}, "", fmt.Errorf("at least one environment is required")
	}

	absTemplate, err := filepath.Abs(templatePath)
	if err != nil {
		return config.GeneratorConfig{}, "", err
	}
	if _, err := os.Stat(absTemplate); err != nil {
		return config.GeneratorConfig{}, "", err
	}

	projectDir := filepath.Dir(absTemplate)
	projectName = normalizeProjectName(projectName, projectDir)
	relTemplate, err := filepath.Rel(projectDir, absTemplate)
	if err != nil {
		relTemplate = absTemplate
	}

	cfg := config.GeneratorConfig{
		App: config.AppConfig{
			Name: projectName,
			Tag:  envs[0].Name,
		},
		Environments: envs,
		Paths: config.PathsConfig{
			SamTemplate: relTemplate,
			OutputDir:   ".esb/",
		},
	}

	generatorPath := filepath.Join(projectDir, "generator.yml")
	return cfg, generatorPath, nil
}

func normalizeProjectName(value, projectDir string) string {
	name := strings.TrimSpace(value)
	if name != "" {
		return name
	}
	return filepath.Base(projectDir)
}

func normalizeEnvs(envs []string) []string {
	cleaned := make([]string, 0, len(envs))
	for _, env := range envs {
		trimmed := strings.TrimSpace(env)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}

func parseEnvSpecs(envs []string) (config.Environments, error) {
	defaultMode := defaultMode()
	specs := make(config.Environments, 0, len(envs))
	for _, env := range envs {
		name, mode := splitEnvMode(env)
		if name == "" {
			continue
		}
		if mode == "" {
			mode = defaultMode
		}
		specs = append(specs, config.EnvironmentSpec{Name: name, Mode: mode})
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("no environments provided")
	}
	return specs, nil
}

func splitEnvMode(value string) (string, string) {
	parts := strings.SplitN(value, ":", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return name, ""
	}
	return name, strings.TrimSpace(parts[1])
}

func defaultMode() string {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("ESB_MODE")))
	if mode == "" {
		return "docker"
	}
	return mode
}
