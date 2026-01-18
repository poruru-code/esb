// Where: cli/internal/config/global.go
// What: Global config load/save helpers.
// Why: Manage ~/.esb/config.yaml consistently.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/meta"
	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the ~/.esb/config.yaml global configuration.
// It tracks registered project paths and last usage.
type GlobalConfig struct {
	Version  int                     `yaml:"version"`
	RepoPath string                  `yaml:"repo_path,omitempty"`
	Projects map[string]ProjectEntry `yaml:"projects,omitempty"`
}

// ProjectEntry stores a project's directory path and last-used timestamp.
type ProjectEntry struct {
	Path     string `yaml:"path"`
	LastUsed string `yaml:"last_used"`
}

// DefaultGlobalConfig returns an initialized GlobalConfig with version set.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Version:  1,
		Projects: map[string]ProjectEntry{},
	}
}

// GlobalConfigPath returns the path to the global config file.
// Respects brand-specific CONFIG_PATH and CONFIG_HOME environment variables.
func GlobalConfigPath() (string, error) {
	if override := strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixConfigPath)); override != "" {
		path := override
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
		}
		return path, nil
	}
	if override := strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixConfigHome)); override != "" {
		return filepath.Join(override, "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, meta.HomeDir, "config.yaml"), nil
}

// EnsureGlobalConfig creates the global config file if it doesn't exist.
func EnsureGlobalConfig() error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return SaveGlobalConfig(path, DefaultGlobalConfig())
		}
		return err
	}
	return nil
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
