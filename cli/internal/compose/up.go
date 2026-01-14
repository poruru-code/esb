// Where: cli/internal/compose/up.go
// What: Docker compose command helpers for bringing stacks up.
// Why: Provide a minimal, testable interface for starting services.
package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ModeContainerd  = "containerd"
	ModeDocker      = "docker"
	ModeFirecracker = "firecracker"
)

// UpOptions contains configuration for starting Docker Compose services.
// It specifies the project, mode, detach settings, and optional build flag.
type UpOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Detach     bool
	Build      bool
	ExtraFiles []string
	EnvFile    string
}

// CommandRunner defines the interface for executing shell commands.
// Implementations run docker compose commands in the specified directory.
type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
	RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// ExecRunner implements CommandRunner using os/exec.
type ExecRunner struct{}

// Run executes a command with inherited stdout/stderr.
func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunOutput executes a command and returns its stdout. Stderr is inherited.
func (ExecRunner) RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// UpProject runs docker compose up with the appropriate configuration
// files for the specified mode and target.
func UpProject(ctx context.Context, runner CommandRunner, opts UpOptions) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}

	mode := resolveMode(opts.Mode)
	args, err := buildComposeArgs(opts.RootDir, mode, opts.Target, opts.Project, opts.ExtraFiles)
	if err != nil {
		return err
	}

	if opts.EnvFile != "" {
		args = append(args, "--env-file", opts.EnvFile)
	}

	args = append(args, "up")
	if opts.Detach {
		args = append(args, "-d")
	}
	if opts.Build {
		args = append(args, "--build")
	}

	return runner.Run(ctx, opts.RootDir, "docker", args...)
}

// ResolveComposeFiles returns the list of docker-compose files to use
// based on the mode (docker/containerd/firecracker) and target.
func ResolveComposeFiles(rootDir, mode, target string) ([]string, error) {
	base := []string{
		filepath.Join(rootDir, "docker-compose.yml"),
		filepath.Join(rootDir, "docker-compose.worker.yml"),
	}

	var extra []string
	switch resolveMode(mode) {
	case ModeFirecracker:
		extra = []string{
			filepath.Join(rootDir, "docker-compose.registry.yml"),
			filepath.Join(rootDir, "docker-compose.fc.yml"),
		}
	case ModeContainerd:
		extra = []string{
			filepath.Join(rootDir, "docker-compose.registry.yml"),
			filepath.Join(rootDir, "docker-compose.containerd.yml"),
		}
	default:
		extra = []string{
			filepath.Join(rootDir, "docker-compose.docker.yml"),
		}
	}

	files := append(base, extra...)
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("compose file not found: %s", path)
		}
	}

	if target != "" && target != "control" {
		return nil, fmt.Errorf("unsupported compose target: %s", target)
	}

	return files, nil
}

// resolveMode normalizes the mode string, falling back to ESB_MODE env
// variable or "docker" default.
func resolveMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case ModeContainerd, ModeDocker, ModeFirecracker:
		return normalized
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ESB_MODE")))
	switch env {
	case ModeContainerd, ModeDocker, ModeFirecracker:
		return env
	default:
		return ModeDocker
	}
}
