// Where: cli/internal/app/selector.go
// What: Interactive selection helpers using huh library.
// Why: Provide keyboard-based selection for env/project commands.
package app

import (
	"os"
	"path/filepath"
	"strings"

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

func (p HuhPrompter) InputPath(title string) (string, error) {
	var input string
	err := huh.NewInput().
		Title(title).
		Description("(Tab, →, or Ctrl+E to complete path, Enter to confirm)").
		SuggestionsFunc(func() []string {
			expandTilde := func(path string) string {
				if strings.HasPrefix(path, "~/") {
					home, _ := os.UserHomeDir()
					return filepath.Join(home, path[2:])
				}
				if path == "~" {
					home, _ := os.UserHomeDir()
					return home
				}
				return path
			}

			// Base directory and prefix for searching
			dir := "."
			prefix := ""
			p := expandTilde(input)

			if input != "" {
				if strings.HasSuffix(input, "/") {
					dir = p
					prefix = ""
				} else {
					dir = filepath.Dir(p)
					prefix = filepath.Base(p)
					if dir == "" {
						dir = "."
					}
				}
			}

			// List files in the resolved directory
			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil
			}

			// Track prefix of the original input to reconstruct suggestions
			originalInputPrefix := ""
			if input != "" {
				if strings.HasSuffix(input, "/") {
					originalInputPrefix = input
				} else {
					lastSlash := strings.LastIndex(input, "/")
					if lastSlash >= 0 {
						originalInputPrefix = input[:lastSlash+1]
					}
				}
			}

			var matches []string
			for _, e := range entries {
				name := e.Name()
				// Case-insensitive filtering for better UX
				if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
					continue
				}

				suggestion := originalInputPrefix + name
				if e.IsDir() {
					suggestion += "/"
				}
				matches = append(matches, suggestion)
			}
			return matches
		}, &input).
		Value(&input).
		Run()
	if err != nil {
		return "", err
	}

	// Final expansion before returning
	if strings.HasPrefix(input, "~/") {
		home, _ := os.UserHomeDir()
		input = filepath.Join(home, input[2:])
	} else if input == "~" {
		home, _ := os.UserHomeDir()
		input = home
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

// selectOption represents a single option in a selection menu.
type selectOption struct {
	Label string // Display text
	Value string // Return value
}
