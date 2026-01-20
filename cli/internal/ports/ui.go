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

// NewConsoleUI returns a UserInterface backed by the console helper.
func NewConsoleUI(out io.Writer) UserInterface {
	return consoleUI{console: ui.New(out)}
}

// NewLegacyUI returns a UserInterface that preserves legacy CLI output.
func NewLegacyUI(out io.Writer) UserInterface {
	return legacyUI{
		out:     out,
		console: ui.New(out),
	}
}

type consoleUI struct {
	console *ui.Console
}

func (c consoleUI) Info(msg string) {
	c.console.Info(msg)
}

func (c consoleUI) Warn(msg string) {
	c.console.Warn(msg)
}

func (c consoleUI) Success(msg string) {
	c.console.Success(msg)
}

func (c consoleUI) Block(emoji, title string, rows []KeyValue) {
	c.console.BlockStart(emoji, title)
	for _, kv := range rows {
		c.console.Item(kv.Key, kv.Value)
	}
	c.console.BlockEnd()
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
