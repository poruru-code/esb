// Where: tools-go/internal/config/global.go
// What: Global config load/save helpers.
// Why: Manage ~/.esb/config.yaml consistently.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type GlobalConfig struct {
	Version            int                     `yaml:"version"`
	ActiveProject      string                  `yaml:"active_project"`
	ActiveEnvironments map[string]string       `yaml:"active_environments,omitempty"`
	Projects           map[string]ProjectEntry `yaml:"projects,omitempty"`
}

type ProjectEntry struct {
	Path     string `yaml:"path"`
	LastUsed string `yaml:"last_used"`
}

func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".esb", "config.yaml"), nil
}

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
