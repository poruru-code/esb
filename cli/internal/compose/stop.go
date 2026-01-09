// Where: cli/internal/compose/stop.go
// What: Docker compose helpers for stop/logs commands.
// Why: Provide a testable interface for stack stop/log access.
package compose

import (
	"context"
	"fmt"
	"strings"
)

type StopOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Services   []string
	ExtraFiles []string
}

type LogsOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Follow     bool
	Tail       int
	Timestamps bool
	Service    string
	ExtraFiles []string
}

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

func LogsProject(ctx context.Context, runner CommandRunner, opts LogsOptions) error {
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

	args = append(args, "logs")
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", opts.Tail))
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	if strings.TrimSpace(opts.Service) != "" {
		args = append(args, opts.Service)
	}

	return runner.Run(ctx, opts.RootDir, "docker", args...)
}
