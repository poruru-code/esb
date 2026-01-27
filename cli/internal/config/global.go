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
	Version       int                      `yaml:"version"`
	RepoPath      string                   `yaml:"repo_path,omitempty"`
	Projects      map[string]ProjectEntry  `yaml:"projects,omitempty"`
	BuildDefaults map[string]BuildDefaults `yaml:"build_defaults,omitempty"`
}

// ProjectEntry stores a project's directory path and last-used timestamp.
type ProjectEntry struct {
	Path     string `yaml:"path"`
	LastUsed string `yaml:"last_used"`
}

// BuildDefaults stores last-used build inputs for a template.
type BuildDefaults struct {
	Env       string            `yaml:"env,omitempty"`
	Mode      string            `yaml:"mode,omitempty"`
	OutputDir string            `yaml:"output_dir,omitempty"`
	Params    map[string]string `yaml:"params,omitempty"`
}

// DefaultGlobalConfig returns an initialized GlobalConfig with version set.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Version:       1,
		Projects:      map[string]ProjectEntry{},
		BuildDefaults: map[string]BuildDefaults{},
	}
}

// GlobalConfigPath returns the path to the global config file.
// Respects brand-specific CONFIG_PATH and CONFIG_HOME environment variables.
func GlobalConfigPath() (string, error) {
	override, err := envutil.GetHostEnv(constants.HostSuffixConfigPath)
	if err != nil {
		return "", err
	}
	if override := strings.TrimSpace(override); override != "" {
		path := override
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
		}
		return path, nil
	}
	override, err = envutil.GetHostEnv(constants.HostSuffixConfigHome)
	if err != nil {
		return "", err
	}
	if override := strings.TrimSpace(override); override != "" {
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
