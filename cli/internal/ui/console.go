// Where: cli/internal/ui/console.go
// What: Console output helpers for consistent CLI UX.
// Why: Standardize emojis, indentation, and structure across commands.
package ui

import (
	"fmt"
	"io"
)

// Console provides helper methods for formatted output.
type Console struct {
	Out io.Writer
}

// New creates a new Console writing to the provided writer.
func New(out io.Writer) *Console {
	return &Console{Out: out}
}

// Header prints a section header with an emoji.
// Example: 🔑 Authentication credentials:
func (c *Console) Header(emoji, title string) {
	fmt.Fprintf(c.Out, "%s %s\n", emoji, title)
}

// Item prints a key-value item with indentation.
// Example:    Key: Value
func (c *Console) Item(key string, value any) {
	fmt.Fprintf(c.Out, "   %-18s %v\n", key+":", value)
}

// ItemPlain prints a generic indented line.
// Example:    Simple message
func (c *Console) ItemPlain(msg string) {
	fmt.Fprintf(c.Out, "   %s\n", msg)
}

// Success prints a success message with a checkmark.
func (c *Console) Success(msg string) {
	fmt.Fprintf(c.Out, "✅ %s\n", msg)
}

// Info prints an info message with an arrow.
func (c *Console) Info(msg string) {
	fmt.Fprintf(c.Out, "➜ %s\n", msg)
}
