// Where: cli/internal/compose/build.go
// What: Docker compose build helpers.
// Why: Build gateway/agent images in a consistent way.
package compose

import (
	"context"
	"fmt"
	"strings"
)

type BuildOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Services   []string
	ExtraFiles []string
	NoCache    bool
}

func BuildProject(ctx context.Context, runner CommandRunner, opts BuildOptions) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if opts.RootDir == "" {
		return fmt.Errorf("root dir is required")
	}

	mode := resolveMode(opts.Mode)
	services := append([]string{}, opts.Services...)
	if mode == ModeContainerd || mode == ModeFirecracker {
		services = ensureService(services, "runtime-node")
	}
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

	args = append(args, "build")
	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	if len(services) > 0 {
		args = append(args, services...)
	}

	return runner.Run(ctx, opts.RootDir, "docker", args...)
}

func ensureService(services []string, name string) []string {
	for _, service := range services {
		if service == name {
			return services
		}
	}
	return append(services, name)
}
