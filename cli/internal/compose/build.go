// Where: cli/internal/compose/build.go
// What: Docker compose build helpers.
// Why: Build gateway/agent images in a consistent way.
package compose

import (
	"context"
	"fmt"
	"os"
)

// BuildOptions contains configuration for building Docker Compose services.
// It specifies the project, mode, services to build, and cache settings.
type BuildOptions struct {
	RootDir    string
	Project    string
	Mode       string
	Target     string
	Services   []string
	ExtraFiles []string
	NoCache    bool
	Verbose    bool
}

// BuildProject runs docker compose build with the appropriate configuration
// files for the specified mode and target. Automatically includes runtime-node
// for containerd/firecracker modes.
func BuildProject(ctx context.Context, runner CommandRunner, opts BuildOptions) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}

	mode := resolveMode(opts.Mode)
	services := append([]string{}, opts.Services...)
	if mode == ModeContainerd || mode == ModeFirecracker {
		services = ensureService(services, "runtime-node")
	}

	args, err := buildComposeArgs(opts.RootDir, mode, opts.Target, opts.Project, opts.ExtraFiles)
	if err != nil {
		return err
	}

	args = append(args, "build")
	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	if len(services) > 0 {
		args = append(args, services...)
	}

	if opts.Verbose {
		return runner.Run(ctx, opts.RootDir, "docker", args...)
	}
	output, err := runner.RunOutput(ctx, opts.RootDir, "docker", args...)
	if err != nil {
		if len(output) > 0 {
			_, _ = os.Stderr.Write(output)
		}
		return fmt.Errorf("compose build failed: %w", err)
	}
	return nil
}

// ensureService adds a service to the list if not already present.
func ensureService(services []string, name string) []string {
	for _, service := range services {
		if service == name {
			return services
		}
	}
	return append(services, name)
}
