// Where: cli/internal/ports/ui.go
// What: User interface abstraction for workflows.
// Why: Provide a single output surface so workflows stay UI-agnostic.
package ports

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/ui"
)

// KeyValue is a key/value pair rendered inside a block.
type KeyValue struct {
	Key   string
	Value any
}

// UserInterface exposes high-level output helpers used by workflows.
type UserInterface interface {
	Info(msg string)
	Warn(msg string)
	Success(msg string)
	Block(emoji, title string, rows []KeyValue)
}

// NewLegacyUI returns a UserInterface that preserves legacy CLI output.
func NewLegacyUI(out io.Writer) UserInterface {
	return legacyUI{
		out:     out,
		console: ui.New(out),
	}
}

type legacyUI struct {
	out     io.Writer
	console *ui.Console
}

func (l legacyUI) Info(msg string) {
	fmt.Fprintln(l.out, msg)
}

func (l legacyUI) Warn(msg string) {
	fmt.Fprintln(l.out, msg)
}

func (l legacyUI) Success(msg string) {
	fmt.Fprintln(l.out, msg)
}

func (l legacyUI) Block(emoji, title string, rows []KeyValue) {
	l.console.BlockStart(emoji, title)
	for _, kv := range rows {
		l.console.Item(kv.Key, kv.Value)
	}
	l.console.BlockEnd()
}
