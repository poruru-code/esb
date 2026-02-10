// Where: cli/internal/command/deploy_template_resolve.go
// What: Template path resolution and interactive selection flow.
// Why: Keep template selection flow independent from path/prompt helpers.
package command

import (
	"fmt"
	"os"
	"strings"

	domaintpl "github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

func resolveDeployTemplate(
	value string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return normalizeTemplatePath(trimmed)
	}
	if !isTTY || prompter == nil {
		return "", errTemplatePathRequired
	}
	for {
		history := loadTemplateHistory()
		candidates := discoverTemplateCandidates()
		suggestions := domaintpl.BuildSuggestions(previous, history, candidates)
		defaultValue := ""
		if len(suggestions) > 0 {
			defaultValue = suggestions[0]
		}
		title := "Template path"
		if defaultValue != "" {
			title = fmt.Sprintf("Template path (default: %s)", defaultValue)
		}

		if len(history) > 0 || len(suggestions) > 0 {
			options := append([]string{}, suggestions...)
			options = append(options, templateManualOption)
			selected, err := prompter.Select(title, options)
			if err != nil {
				return "", fmt.Errorf("prompt template selection: %w", err)
			}
			if selected == templateManualOption {
				input, err := prompter.Input(title, suggestions)
				if err != nil {
					return "", fmt.Errorf("prompt template path: %w", err)
				}
				input = strings.TrimSpace(input)
				if input == "" {
					if defaultValue != "" {
						input = defaultValue
					} else if path, err := resolveTemplateFallback(previous, candidates); err == nil {
						return path, nil
					} else {
						fmt.Fprintln(os.Stderr, "Template path is required.")
						continue
					}
				}
				path, err := normalizeTemplatePath(input)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
					continue
				}
				return path, nil
			}
			path, err := normalizeTemplatePath(selected)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
				continue
			}
			return path, nil
		}

		input, err := prompter.Input(title, suggestions)
		if err != nil {
			return "", fmt.Errorf("prompt template path: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			if defaultValue != "" {
				input = defaultValue
			} else if path, err := resolveTemplateFallback(previous, candidates); err == nil {
				return path, nil
			} else {
				fmt.Fprintln(os.Stderr, "Template path is required.")
				continue
			}
		}
		path, err := normalizeTemplatePath(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
			continue
		}
		return path, nil
	}
}

func resolveDeployTemplates(
	values []string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) ([]string, error) {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if v := strings.TrimSpace(value); v != "" {
			trimmed = append(trimmed, v)
		}
	}
	if len(trimmed) > 0 {
		out := make([]string, 0, len(trimmed))
		for _, value := range trimmed {
			path, err := normalizeTemplatePath(value)
			if err != nil {
				return nil, err
			}
			out = append(out, path)
		}
		return out, nil
	}
	path, err := resolveDeployTemplate("", isTTY, prompter, previous)
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}
