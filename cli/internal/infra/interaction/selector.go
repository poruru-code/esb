// Where: cli/internal/infra/interaction/selector.go
// What: Interactive selection helpers using the huh library.
// Why: Provide keyboard-based selection for env/project commands.
package interaction

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

var runInputPrompt = func(title string, suggestions []string, input *string) error {
	field := huh.NewInput().
		Title(title).
		Suggestions(suggestions).
		Value(input)
	if len(suggestions) > 0 {
		field.Placeholder(suggestions[0])
	}
	return field.Run()
}

var runSelectPrompt = func(title string, options []huh.Option[string], selected *string) error {
	return huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(selected).
		Run()
}

// HuhPrompter implements the Prompter interface using the huh TUI library.
type HuhPrompter struct{}

func (p HuhPrompter) Input(title string, suggestions []string) (string, error) {
	var input string
	err := runInputPrompt(title, suggestions, &input)
	if err != nil {
		return "", fmt.Errorf("prompt input: %w", err)
	}
	return input, nil
}

func (p HuhPrompter) Select(title string, options []string) (string, error) {
	var selected string
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt, opt)
	}

	err := runSelectPrompt(title, huhOptions, &selected)
	if err != nil {
		return "", fmt.Errorf("prompt select: %w", err)
	}
	return selected, nil
}

func (p HuhPrompter) SelectValue(title string, options []SelectOption) (string, error) {
	if len(options) == 0 {
		return "", nil
	}

	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
	}

	var selected string
	err := runSelectPrompt(title, huhOptions, &selected)
	if err != nil {
		return "", fmt.Errorf("prompt select value: %w", err)
	}
	return selected, nil
}
