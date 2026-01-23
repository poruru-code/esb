package compose

import (
	"context"
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
type ExecRunner struct{}

func (r ExecRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r ExecRunner) RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func (r ExecRunner) RunQuiet(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Run()
}
