// Where: cli/internal/app/selector.go
// What: Interactive selection helpers using huh library.
// Why: Provide keyboard-based selection for env/project commands.
package app

import (
	"github.com/charmbracelet/huh"
)

// selectFromList displays an interactive selection menu and returns the selected value.
// Returns error if cancelled or if running in non-interactive mode.
func selectFromList(title string, options []selectOption) (string, error) {
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

// selectOption represents a single option in a selection menu.
type selectOption struct {
	Label string // Display text
	Value string // Return value
}
