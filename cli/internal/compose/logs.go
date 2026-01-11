package compose

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
)

// LogsOptions contains configuration for viewing Docker Compose logs.
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

// LogsProject runs docker compose logs with specified follow/tail options.
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

// ListServices returns a list of services defined in the docker-compose project.
func ListServices(ctx context.Context, runner CommandRunner, opts LogsOptions) ([]string, error) {
	if runner == nil {
		return nil, fmt.Errorf("command runner is nil")
	}
	if opts.RootDir == "" {
		return nil, fmt.Errorf("root dir is required")
	}

	mode := resolveMode(opts.Mode)
	files, err := ResolveComposeFiles(opts.RootDir, mode, opts.Target)
	if err != nil {
		return nil, err
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

	args = append(args, "config", "--services")

	output, err := runner.RunOutput(ctx, opts.RootDir, "docker", args...)
	if err != nil {
		return nil, err
	}

	var services []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			services = append(services, line)
		}
	}
	return services, scanner.Err()
}
