// Where: cli/internal/config/global.go
// What: Global config load/save helpers.
// Why: Manage ~/.esb/config.yaml consistently.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the ~/.esb/config.yaml global configuration.
// It tracks active project, environments, and registered project paths.
type GlobalConfig struct {
	Version            int                     `yaml:"version"`
	ActiveProject      string                  `yaml:"active_project"`
	ActiveEnvironments map[string]string       `yaml:"active_environments,omitempty"`
	Projects           map[string]ProjectEntry `yaml:"projects,omitempty"`
}

// ProjectEntry stores a project's directory path and last-used timestamp.
type ProjectEntry struct {
	Path     string `yaml:"path"`
	LastUsed string `yaml:"last_used"`
}

// GlobalConfigPath returns the path to the global config file.
// Respects ESB_CONFIG_PATH and ESB_CONFIG_HOME environment variables.
func GlobalConfigPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("ESB_CONFIG_PATH")); override != "" {
		path := override
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
		}
		return path, nil
	}
	if override := strings.TrimSpace(os.Getenv("ESB_CONFIG_HOME")); override != "" {
		return filepath.Join(override, "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".esb", "config.yaml"), nil
}

// LoadGlobalConfig reads and parses the global configuration file.
func LoadGlobalConfig(path string) (GlobalConfig, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return GlobalConfig{}, err
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return GlobalConfig{}, err
	}
	return cfg, nil
}

// SaveGlobalConfig writes a GlobalConfig to the specified path.
func SaveGlobalConfig(path string, cfg GlobalConfig) error {
	payload, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}
