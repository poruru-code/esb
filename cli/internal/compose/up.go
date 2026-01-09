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

type UpOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Detach     bool
	Build      bool
	ExtraFiles []string
}

type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func UpProject(ctx context.Context, runner CommandRunner, opts UpOptions) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if opts.RootDir == "" {
		return fmt.Errorf("root dir is required")
	}

	mode := resolveMode(opts.Mode)
	files, err := ResolveComposeFiles(opts.RootDir, mode, opts.Target)
	if err != nil {
		return err
	}

	args := []string{"compose"}
	if opts.Project != "" {
		args = append(args, "-p", opts.Project)
	}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	for _, file := range opts.ExtraFiles {
		if strings.TrimSpace(file) == "" {
			continue
		}
		args = append(args, "-f", file)
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

func FindRepoRoot(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		if dir == "" || dir == string(filepath.Separator) {
			break
		}
		path := filepath.Join(dir, "docker-compose.yml")
		if _, err := os.Stat(path); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("compose root not found from %s", start)
}
