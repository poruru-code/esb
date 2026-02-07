// Where: cli/internal/infra/ui/console.go
// What: Console output helpers for consistent CLI UX.
// Why: Standardize emojis, indentation, and structure across commands.
package ui

import (
	"fmt"
	"io"
	"strings"
)

// Console provides helper methods for formatted output.
type Console struct {
	Out          io.Writer
	EmojiEnabled bool
}

// New creates a new Console writing to the provided writer.
func New(out io.Writer) *Console {
	return &Console{Out: out, EmojiEnabled: true}
}

// NewWithEmoji creates a new Console with explicit emoji settings.
func NewWithEmoji(out io.Writer, enabled bool) *Console {
	return &Console{Out: out, EmojiEnabled: enabled}
}

// Header prints a section header with an emoji.
// Example: 🔑 Authentication credentials.
func (c *Console) Header(emoji, title string) {
	fmt.Fprintf(c.Out, "%s%s\n", c.emojiPrefix(emoji), title)
}

// BlockStart starts a logical block of information with an emoji header.
// It ensures there is vertical padding (blank line) around the block for better parsing.
func (c *Console) BlockStart(emoji, title string) {
	// Depending on UX preference, could enforce pre-padding, but post-padding is key for "End of Block".
	// Let's enforce pre-padding to separate from previous logs.
	fmt.Fprintln(c.Out)
	c.Header(emoji, title)
}

// BlockEnd ends a logical block.
// It prints a blank line to clearly demarcate the end of the information block.
func (c *Console) BlockEnd() {
	fmt.Fprintln(c.Out)
}

// Item prints a key-value item with indentation.
// Example:    Key: Value.
func (c *Console) Item(key string, value any) {
	fmt.Fprintf(c.Out, "   %-30s %v\n", key+":", value)
}

// ItemPlain prints a generic indented line.
// Example:    Simple message.
func (c *Console) ItemPlain(msg string) {
	fmt.Fprintf(c.Out, "   %s\n", msg)
}

// Success prints a success message with a checkmark.
func (c *Console) Success(msg string) {
	prefix := c.emojiPrefix("✅")
	if prefix == "" {
		prefix = "[ok] "
	}
	fmt.Fprintf(c.Out, "%s%s\n", prefix, msg)
}

// Info prints an info message.
func (c *Console) Info(msg string) {
	fmt.Fprintf(c.Out, "%s\n", msg)
}

// Warn prints a warning message with an emoji.
func (c *Console) Warn(msg string) {
	prefix := c.emojiPrefix("⚠️")
	if prefix == "" {
		prefix = "[warn] "
	}
	fmt.Fprintf(c.Out, "%s%s\n", prefix, msg)
}

func (c *Console) emojiPrefix(emoji string) string {
	if !c.EmojiEnabled || strings.TrimSpace(emoji) == "" {
		return ""
	}
	return emoji + " "
}
