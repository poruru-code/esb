// Where: cli/internal/compose/stop.go
// What: Docker compose helpers for stop/logs commands.
// Why: Provide a testable interface for stack stop/log access.
package compose

import (
	"context"
	"fmt"
	"strings"
)

// StopOptions contains configuration for stopping Docker Compose services.
type StopOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Services   []string
	ExtraFiles []string
}

// StopProject runs docker compose stop for the specified project and services.
func StopProject(ctx context.Context, runner CommandRunner, opts StopOptions) error {
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

	args = append(args, "stop")
	if len(opts.Services) > 0 {
		args = append(args, opts.Services...)
	}

	return runner.Run(ctx, opts.RootDir, "docker", args...)
}
