// Where: cli/internal/interaction/selector.go
// What: Interactive selection helpers using the huh library.
// Why: Provide keyboard-based selection for env/project commands.
package interaction

import (
	"github.com/charmbracelet/huh"
)

// HuhPrompter implements the Prompter interface using the huh TUI library.
type HuhPrompter struct{}

func (p HuhPrompter) Input(title string, suggestions []string) (string, error) {
	var input string
	err := huh.NewInput().
		Title(title).
		Suggestions(suggestions).
		Value(&input).
		Run()
	if err != nil {
		return "", err
	}
	return input, nil
}

func (p HuhPrompter) Select(title string, options []string) (string, error) {
	var selected string
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt, opt)
	}

	err := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
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
	err := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}
