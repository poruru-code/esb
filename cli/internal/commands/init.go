// Where: cli/internal/commands/init.go
// What: Init command helpers.
// Why: Build and persist generator.yml for new projects.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/meta"
	"gopkg.in/yaml.v3"
)

// runInit creates a new generator.yml configuration file for a SAM template.
// Returns the path to the generated configuration file.
func runInit(templatePath string, envs []string, projectName string, prompter interaction.Prompter) (string, error) {
	cleaned := normalizeEnvs(envs)
	if len(cleaned) == 0 {
		return "", fmt.Errorf("environment name is required")
	}

	specs, err := parseEnvSpecs(cleaned)
	if err != nil {
		return "", err
	}

	cfg, generatorPath, err := buildGeneratorConfig(templatePath, specs, projectName, prompter)
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
func buildGeneratorConfig(templatePath string, envs config.Environments, projectName string, prompter interaction.Prompter) (config.GeneratorConfig, string, error) {
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
	info, err := os.Stat(absTemplate)
	if err != nil {
		return config.GeneratorConfig{}, "", err
	}

	if info.IsDir() {
		// Try to auto-detect standard template files within the directory
		found := false
		for _, name := range []string{"template.yaml", "template.yml"} {
			chkPath := filepath.Join(absTemplate, name)
			if stat, err := os.Stat(chkPath); err == nil && !stat.IsDir() {
				absTemplate = chkPath
				found = true
				break
			}
		}
		if !found {
			return config.GeneratorConfig{}, "", fmt.Errorf("no template.yaml or template.yml found in directory: %s", templatePath)
		}
	}

	// Parse template parameters
	userParams := make(map[string]any)
	if prompter != nil {
		params, err := parseTemplateParameters(absTemplate)
		if err == nil && len(params) > 0 {
			legacyUI(os.Stdout).Info("Template Parameters:")
			for name, p := range params {
				label := fmt.Sprintf("%s (%s)", name, p.Description)
				if p.Description == "" {
					label = name
				}

				// Safely format default value (which can be nil, string, int, etc.)
				var defaultStr string
				hasDefault := p.Default != nil

				if hasDefault {
					defaultStr = fmt.Sprintf("%v", p.Default)
				}

				var inputTitle string
				if hasDefault {
					displayDefault := defaultStr
					if displayDefault == "" {
						displayDefault = "''" // Explicitly show empty string
					}
					inputTitle = fmt.Sprintf("%s [Default: %s]", label, displayDefault)
				} else {
					inputTitle = fmt.Sprintf("%s [Required]", label)
				}

				// Prompter.Input unfortunately doesn't support pre-filled value easily unless we change interface
				// or just rely on "empty means default".
				val, err := prompter.Input(inputTitle, nil)
				if err != nil {
					return config.GeneratorConfig{}, "", err
				}

				val = strings.TrimSpace(val)
				if val == "" {
					if hasDefault {
						val = defaultStr
					}
					// If no default and empty input, we accept empty string (val is "")
				}

				userParams[name] = val
			}
		} else if err != nil {
			// Warn but don't fail?
			legacyUI(os.Stderr).Warn(fmt.Sprintf("Warning: failed to parse template parameters: %v", err))
		}
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
			OutputDir:   meta.OutputDir + "/",
		},
		Parameters: userParams,
	}

	generatorPath := filepath.Join(projectDir, "generator.yml")
	return cfg, generatorPath, nil
}

type samTemplate struct {
	Parameters map[string]samParameter `yaml:"Parameters"`
}

type samParameter struct {
	Type        string `yaml:"Type"`
	Default     any    `yaml:"Default"` // Default can be string, number, or list
	Description string `yaml:"Description"`
}

func parseTemplateParameters(path string) (map[string]samParameter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t samTemplate
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return t.Parameters, nil
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

// defaultMode returns the default container runtime mode from host environment variable,
// falling back to "docker" if not set.
func defaultMode() string {
	mode := strings.TrimSpace(strings.ToLower(envutil.GetHostEnv(constants.HostSuffixMode)))
	if mode == "" {
		return "docker"
	}
	return mode
}
