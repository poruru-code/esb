// Where: cli/internal/infra/ui/deploy.go
// What: Emoji-aware UI for deploy output.
// Why: Keep deploy output readable while allowing emoji to be toggled.
package ui

import (
	"fmt"
	"io"
)

// NewDeployUI returns a UserInterface tailored for deploy output.
func NewDeployUI(out io.Writer, emojiEnabled bool) UserInterface {
	return deployUI{
		out:     out,
		console: NewWithEmoji(out, emojiEnabled),
	}
}

type deployUI struct {
	out     io.Writer
	console *Console
}

func (d deployUI) Info(msg string) {
	fmt.Fprintln(d.out, msg)
}

func (d deployUI) Warn(msg string) {
	d.console.Warn(msg)
}

func (d deployUI) Success(msg string) {
	d.console.Success(msg)
}

func (d deployUI) Block(emoji, title string, rows []KeyValue) {
	d.console.BlockStart(emoji, title)
	for _, kv := range rows {
		d.console.Item(kv.Key, kv.Value)
	}
	d.console.BlockEnd()
}
