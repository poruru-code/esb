// Where: cli/internal/compose/stop.go
// What: Docker compose helpers for stop/logs commands.
// Why: Provide a testable interface for stack stop/log access.
package compose

import (
	"context"
	"fmt"
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

	mode := resolveMode(opts.Mode)
	args, err := buildComposeArgs(opts.RootDir, mode, opts.Target, opts.Project, opts.ExtraFiles)
	if err != nil {
		return err
	}

	args = append(args, "stop")
	if len(opts.Services) > 0 {
		args = append(args, opts.Services...)
	}

	return runner.Run(ctx, opts.RootDir, "docker", args...)
}
