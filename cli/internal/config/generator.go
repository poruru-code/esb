// Where: cli/internal/config/generator.go
// What: generator.yml load/save helpers.
// Why: Centralize config parsing for CLI commands.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type GeneratorConfig struct {
	App          AppConfig      `yaml:"app"`
	Environments Environments   `yaml:"environments"`
	Paths        PathsConfig    `yaml:"paths"`
	Parameters   map[string]any `yaml:"parameters,omitempty"`
}

type AppConfig struct {
	Name string `yaml:"name"`
	Tag  string `yaml:"tag"`
}

type PathsConfig struct {
	SamTemplate  string `yaml:"sam_template"`
	OutputDir    string `yaml:"output_dir"`
	FunctionsYml string `yaml:"functions_yml,omitempty"`
	RoutingYml   string `yaml:"routing_yml,omitempty"`
}

type EnvironmentSpec struct {
	Name string
	Mode string
}

type Environments []EnvironmentSpec

func (e *Environments) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*e = nil
		return nil
	}

	switch node.Kind {
	case yaml.SequenceNode:
		envs := make(Environments, 0, len(node.Content))
		for _, item := range node.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				envs = append(envs, EnvironmentSpec{Name: strings.TrimSpace(item.Value)})
			case yaml.MappingNode:
				spec, err := parseEnvironmentMapping(item)
				if err != nil {
					return err
				}
				if spec.Name != "" {
					envs = append(envs, spec)
				}
			default:
				return fmt.Errorf("unsupported environment entry")
			}
		}
		*e = envs
		return nil
	case yaml.MappingNode:
		envs := make(Environments, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			value := node.Content[i+1]
			mode := ""
			switch value.Kind {
			case yaml.ScalarNode:
				mode = strings.TrimSpace(value.Value)
			case yaml.MappingNode:
				for j := 0; j+1 < len(value.Content); j += 2 {
					if strings.TrimSpace(value.Content[j].Value) == "mode" {
						mode = strings.TrimSpace(value.Content[j+1].Value)
						break
					}
				}
			default:
				return fmt.Errorf("unsupported environment mode")
			}
			if key == "" {
				continue
			}
			envs = append(envs, EnvironmentSpec{Name: key, Mode: mode})
		}
		*e = envs
		return nil
	default:
		return fmt.Errorf("unsupported environments format")
	}
}

func (e Environments) MarshalYAML() (any, error) {
	out := map[string]string{}
	for _, spec := range e {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		out[name] = strings.TrimSpace(spec.Mode)
	}
	return out, nil
}

func (e Environments) Has(name string) bool {
	_, ok := e.Mode(name)
	return ok
}

func (e Environments) Mode(name string) (string, bool) {
	for _, spec := range e {
		if spec.Name == name {
			return spec.Mode, true
		}
	}
	return "", false
}

func (e Environments) Names() []string {
	names := make([]string, 0, len(e))
	for _, spec := range e {
		if spec.Name == "" {
			continue
		}
		names = append(names, spec.Name)
	}
	return names
}

func parseEnvironmentMapping(node *yaml.Node) (EnvironmentSpec, error) {
	spec := EnvironmentSpec{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := strings.TrimSpace(node.Content[i].Value)
		switch key {
		case "name":
			spec.Name = strings.TrimSpace(node.Content[i+1].Value)
		case "mode":
			spec.Mode = strings.TrimSpace(node.Content[i+1].Value)
		}
	}
	return spec, nil
}

func LoadGeneratorConfig(path string) (GeneratorConfig, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return GeneratorConfig{}, err
	}

	var cfg GeneratorConfig
	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return GeneratorConfig{}, err
	}
	return cfg, nil
}

func SaveGeneratorConfig(path string, cfg GeneratorConfig) error {
	payload, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}
