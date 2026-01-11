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

// runInit creates a new generator.yml configuration file for a SAM template.
// Returns the path to the generated configuration file.
func runInit(templatePath string, envs []string, projectName string) (string, error) {
	cleaned := normalizeEnvs(envs)
	if len(cleaned) == 0 {
		return "", fmt.Errorf("environment name is required")
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

// buildGeneratorConfig constructs the generator.yml configuration structure
// from the provided template path, environments, and project name.
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

// normalizeProjectName returns the provided name if non-empty,
// otherwise derives the project name from the directory basename.
func normalizeProjectName(value, projectDir string) string {
	name := strings.TrimSpace(value)
	if name != "" {
		return name
	}
	return filepath.Base(projectDir)
}

// normalizeEnvs removes empty strings and whitespace from the environment list.
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

// parseEnvSpecs converts environment strings (with optional mode suffix)
// into structured EnvironmentSpec objects.
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

// splitEnvMode splits an environment specifier like "prod:containerd"
// into its name and mode components.
func splitEnvMode(value string) (string, string) {
	parts := strings.SplitN(value, ":", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return name, ""
	}
	return name, strings.TrimSpace(parts[1])
}

// defaultMode returns the default container runtime mode from ESB_MODE
// environment variable, falling back to "docker" if not set.
func defaultMode() string {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("ESB_MODE")))
	if mode == "" {
		return "docker"
	}
	return mode
}
