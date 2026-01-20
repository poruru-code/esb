// Where: cli/internal/commands/mock_prompter_test.go
// What: Test helper prompter for interaction-dependent command tests.
// Why: Provide deterministic input/select behavior without TTY.
package commands

import "github.com/poruru/edge-serverless-box/cli/internal/interaction"

type mockPrompter struct {
	inputFn       func(title string, suggestions []string) (string, error)
	selectFn      func(title string, options []string) (string, error)
	selectValueFn func(title string, options []interaction.SelectOption) (string, error)

	// Convenience fields for recording/controlling selections
	selectedValue string
	lastTitle     string
	lastOptions   []string
}

func (m *mockPrompter) Input(title string, suggestions []string) (string, error) {
	m.lastTitle = title
	if m.inputFn != nil {
		return m.inputFn(title, suggestions)
	}
	return m.selectedValue, nil
}

func (m *mockPrompter) Select(title string, options []string) (string, error) {
	m.lastTitle = title
	m.lastOptions = options
	if m.selectFn != nil {
		return m.selectFn(title, options)
	}
	return m.selectedValue, nil
}

func (m *mockPrompter) SelectValue(title string, options []interaction.SelectOption) (string, error) {
	m.lastTitle = title
	if m.selectValueFn != nil {
		return m.selectValueFn(title, options)
	}
	return m.selectedValue, nil
}
