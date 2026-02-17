// Where: cli/internal/infra/config/global.go
// What: Global config load/save env.
// Why: Manage <repo_root>/.<brand>/config.yaml consistently.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/meta"
	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the <repo_root>/.<brand>/config.yaml project configuration.
// It tracks registered project paths and last usage.
type GlobalConfig struct {
	Version         int                      `yaml:"version"`
	RepoPath        string                   `yaml:"repo_path,omitempty"`
	Projects        map[string]ProjectEntry  `yaml:"projects,omitempty"`
	BuildDefaults   map[string]BuildDefaults `yaml:"build_defaults,omitempty"`
	RecentTemplates []string                 `yaml:"recent_templates,omitempty"`
}

// ProjectEntry stores a project's directory path and last-used timestamp.
type ProjectEntry struct {
	Path     string `yaml:"path"`
	LastUsed string `yaml:"last_used"`
}

// BuildDefaults stores last-used build inputs for a template.
type BuildDefaults struct {
	Env           string            `yaml:"env,omitempty"`
	Mode          string            `yaml:"mode,omitempty"`
	OutputDir     string            `yaml:"output_dir,omitempty"`
	Params        map[string]string `yaml:"params,omitempty"`
	ImageSources  map[string]string `yaml:"image_sources,omitempty"`
	ImageRuntimes map[string]string `yaml:"image_runtimes,omitempty"`
}

// DefaultGlobalConfig returns an initialized GlobalConfig with version set.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Version:         1,
		Projects:        map[string]ProjectEntry{},
		BuildDefaults:   map[string]BuildDefaults{},
		RecentTemplates: []string{},
	}
}

// ProjectConfigPath returns the path to the project config file.
func ProjectConfigPath(projectRoot string) (string, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return "", fmt.Errorf("project root is required")
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return filepath.Join(root, meta.HomeDir, "config.yaml"), nil
}

// EnsureProjectConfig creates the project config file if it doesn't exist.
func EnsureProjectConfig(projectRoot string) error {
	path, err := ProjectConfigPath(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project config path: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return SaveGlobalConfig(path, DefaultGlobalConfig())
		}
		return fmt.Errorf("stat project config: %w", err)
	}
	return nil
}

// LoadGlobalConfig reads and parses the global configuration file.
func LoadGlobalConfig(path string) (GlobalConfig, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("read global config: %w", err)
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return GlobalConfig{}, fmt.Errorf("decode global config: %w", err)
	}
	return cfg, nil
}

// SaveGlobalConfig writes a GlobalConfig to the specified path.
func SaveGlobalConfig(path string, cfg GlobalConfig) error {
	payload, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("encode global config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create global config dir: %w", err)
	}

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write global config: %w", err)
	}
	return nil
}
