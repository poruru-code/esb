package composeprovision

import (
	"context"
	"fmt"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, cwd, name string, args ...string) error
	RunQuiet(ctx context.Context, cwd, name string, args ...string) error
}

type Request struct {
	ComposeProject  string
	ComposeFiles    []string
	EnvFile         string
	NoDeps          bool
	Verbose         bool
	NoWarnOrphans   bool
	ProvisionerName string
}

func Execute(ctx context.Context, runner Runner, workingDir string, req Request) error {
	if runner == nil {
		return fmt.Errorf("compose runner is not configured")
	}
	args := buildArgs(req)
	if req.Verbose {
		if err := runner.Run(ctx, workingDir, "docker", args...); err != nil {
			return fmt.Errorf("run provisioner: %w", err)
		}
		return nil
	}
	if err := runner.RunQuiet(ctx, workingDir, "docker", args...); err != nil {
		return fmt.Errorf("run provisioner: %w", err)
	}
	return nil
}

func buildArgs(req Request) []string {
	provisionerName := strings.TrimSpace(req.ProvisionerName)
	if provisionerName == "" {
		provisionerName = "provisioner"
	}

	args := []string{"compose"}
	for _, file := range req.ComposeFiles {
		normalized := strings.TrimSpace(file)
		if normalized == "" {
			continue
		}
		args = append(args, "-f", normalized)
	}
	if req.NoWarnOrphans {
		args = append(args, "--no-warn-orphans")
	}
	if project := strings.TrimSpace(req.ComposeProject); project != "" {
		args = append(args, "-p", project)
	}
	if envFile := strings.TrimSpace(req.EnvFile); envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, "--profile", "deploy", "run", "--rm")
	if req.NoDeps {
		args = append(args, "--no-deps")
	}
	args = append(args, provisionerName)
	return args
}
