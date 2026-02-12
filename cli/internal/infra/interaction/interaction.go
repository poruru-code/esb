// Where: cli/internal/infra/interaction/interaction.go
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

	"github.com/mattn/go-isatty"
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
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// PromptYesNo prints a confirmation prompt and returns true for yes.
func PromptYesNo(message string) (bool, error) {
	return PromptYesNoWithIO(os.Stdin, os.Stderr, message)
}

// PromptYesNoWithIO prints a confirmation prompt to out and reads the answer from in.
func PromptYesNoWithIO(in io.Reader, out io.Writer, message string) (bool, error) {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	reader := bufio.NewReader(in)
	_, _ = fmt.Fprintf(out, "%s [y/N]: ", message)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	return trimmed == "y" || trimmed == "yes", nil
}
