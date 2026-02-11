// Where: cli/internal/command/deploy_template_prompt.go
// What: SAM template parameter extraction and prompt handling.
// Why: Keep parameter prompt behavior isolated from template path resolution.
package command

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/sam"
)

func promptTemplateParameters(
	templatePath string,
	isTTY bool,
	prompter interaction.Prompter,
	previous map[string]string,
	errOut io.Writer,
) (map[string]string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return map[string]string{}, fmt.Errorf("read template: %w", err)
	}

	data, err := sam.DecodeYAML(string(content))
	if err != nil {
		return map[string]string{}, fmt.Errorf("decode template: %w", err)
	}

	params := extractSAMParameters(data)
	if len(params) == 0 {
		return map[string]string{}, nil
	}

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	values := make(map[string]string, len(params))
	for _, name := range names {
		param := params[name]
		hasDefault := param.Default != nil
		defaultStr := ""
		if hasDefault {
			defaultStr = fmt.Sprint(param.Default)
		}
		prevValue := ""
		if previous != nil {
			prevValue = strings.TrimSpace(previous[name])
		}

		if !isTTY || prompter == nil {
			if hasDefault {
				values[name] = defaultStr
				continue
			}
			if prevValue != "" {
				values[name] = prevValue
				continue
			}
			return nil, fmt.Errorf("%w: %s", errParameterRequiresValue, name)
		}

		label := name
		if param.Description != "" {
			label = fmt.Sprintf("%s (%s)", name, param.Description)
		}

		var title string
		switch {
		case hasDefault:
			displayDefault := defaultStr
			if displayDefault == "" {
				displayDefault = "''"
			}
			title = fmt.Sprintf("%s [Default: %s]", label, displayDefault)
		case prevValue != "":
			title = fmt.Sprintf("%s [Previous: %s]", label, prevValue)
		default:
			title = fmt.Sprintf("%s [Required]", label)
		}

		suggestions := []string{}
		if prevValue != "" {
			suggestions = append(suggestions, prevValue)
		}
		for {
			input, err := prompter.Input(title, suggestions)
			if err != nil {
				return nil, fmt.Errorf("prompt parameter %s: %w", name, err)
			}
			input = strings.TrimSpace(input)
			if input == "" && hasDefault {
				input = defaultStr
			}
			if input == "" && prevValue != "" {
				input = prevValue
			}
			if input == "" && !hasDefault {
				if strings.EqualFold(param.Type, "String") {
					values[name] = ""
					break
				}
				writeWarningf(errOut, "Parameter %q is required.\n", name)
				continue
			}
			values[name] = input
			break
		}
	}

	return values, nil
}

// extractSAMParameters extracts parameter definitions from SAM template data.
func extractSAMParameters(data map[string]any) map[string]samParameter {
	result := make(map[string]samParameter)
	params := asMap(data["Parameters"])
	if params == nil {
		return result
	}

	for name, val := range params {
		m := asMap(val)
		if m == nil {
			continue
		}

		param := samParameter{}
		if t, ok := m["Type"].(string); ok {
			param.Type = t
		}
		if d, ok := m["Description"].(string); ok {
			param.Description = d
		}
		param.Default = m["Default"]
		result[name] = param
	}

	return result
}

// asMap converts an interface to a map[string]any.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if m, ok := v.(map[any]any); ok {
		result := make(map[string]any, len(m))
		for key, value := range m {
			if sk, ok := key.(string); ok {
				result[sk] = value
			}
		}
		return result
	}
	return nil
}
