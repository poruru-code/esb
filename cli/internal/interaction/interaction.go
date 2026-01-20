// Where: cli/internal/interaction/interaction.go
// What: Interactive primitives for CLI prompts and TTY detection.
// Why: Centralize user interaction to keep command handlers focused on orchestration.
package interaction

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// SelectOption represents a single option in a selection menu.
type SelectOption struct {
	Label string // Display text
	Value string // Return value
}

// Prompter defines the interface for interactive user input and selection.
type Prompter interface {
	Input(title string, suggestions []string) (string, error)
	Select(title string, options []string) (string, error)
	SelectValue(title string, options []SelectOption) (string, error)
}

// IsTerminal reports whether the file refers to a terminal device.
var IsTerminal = func(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	// Check for character device and ensure it's not a pipe or redirect.
	return (info.Mode()&os.ModeCharDevice) != 0 && (fd == 0 || fd == 1 || fd == 2)
}

// PromptYesNo prints a confirmation prompt and returns true for yes.
func PromptYesNo(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", message)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	return trimmed == "y" || trimmed == "yes", nil
}
