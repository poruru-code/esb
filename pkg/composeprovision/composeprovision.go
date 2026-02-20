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
	runCommand := func(args []string) error {
		if req.Verbose {
			return runner.Run(ctx, workingDir, "docker", args...)
		}
		return runner.RunQuiet(ctx, workingDir, "docker", args...)
	}

	if req.NoDeps {
		if err := runCommand(buildArgs(req)); err != nil {
			return fmt.Errorf("run provisioner: build image: %w", err)
		}
	}
	if err := runCommand(runArgs(req)); err != nil {
		return fmt.Errorf("run provisioner: %w", err)
	}
	return nil
}

func buildArgs(req Request) []string {
	provisionerName := strings.TrimSpace(req.ProvisionerName)
	if provisionerName == "" {
		provisionerName = "provisioner"
	}
	args := composeBaseArgs(req)
	args = append(args, "--profile", "deploy", "build", provisionerName)
	return args
}

func runArgs(req Request) []string {
	provisionerName := strings.TrimSpace(req.ProvisionerName)
	if provisionerName == "" {
		provisionerName = "provisioner"
	}
	args := composeBaseArgs(req)
	args = append(args, "--profile", "deploy", "run", "--rm")
	if req.NoDeps {
		args = append(args, "--no-deps")
	}
	args = append(args, provisionerName)
	return args
}

func composeBaseArgs(req Request) []string {
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
	return args
}
