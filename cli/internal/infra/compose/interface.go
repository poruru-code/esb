package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// CommandRunner defines the interface for executing external commands.
type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
	RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error)
	RunQuiet(ctx context.Context, dir, name string, args ...string) error
}

// ExecRunner is a concrete implementation of CommandRunner using os/exec.
type ExecRunner struct {
	Out    io.Writer
	ErrOut io.Writer
}

func (r ExecRunner) stdout() io.Writer {
	if r.Out != nil {
		return r.Out
	}
	return os.Stdout
}

func (r ExecRunner) stderr() io.Writer {
	if r.ErrOut != nil {
		return r.ErrOut
	}
	return os.Stderr
}

func (r ExecRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

func (r ExecRunner) RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run %s: %w", name, err)
	}
	return output, nil
}

func (r ExecRunner) RunQuiet(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}
