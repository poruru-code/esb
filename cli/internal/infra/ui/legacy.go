// Where: cli/internal/infra/ui/legacy.go
// What: Legacy CLI UI adapter for workflows/usecases.
// Why: Provide a stable output surface without the old ports layer.
package ui

import (
	"fmt"
	"io"
)

// KeyValue is a key/value pair rendered inside a block.
type KeyValue struct {
	Key   string
	Value any
}

// UserInterface exposes high-level output helpers used by workflows/usecases.
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
		console: New(out),
	}
}

type legacyUI struct {
	out     io.Writer
	console *Console
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
