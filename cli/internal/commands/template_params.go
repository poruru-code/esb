// Where: cli/internal/commands/template_params.go
// What: SAM template parameter parsing helpers for build-only CLI.
// Why: Support interactive parameter input without generator.yml.
package commands

import (
	"os"

	"gopkg.in/yaml.v3"
)

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
