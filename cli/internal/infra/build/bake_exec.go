// Where: cli/internal/infra/build/bake_exec.go
// What: Bake command execution and argument assembly.
// Why: Isolate bake run flow from HCL/render and builder internals.
package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

func runBakeGroup(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	lockRoot string,
	groupName string,
	targets []bakeTarget,
	verbose bool,
) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if strings.TrimSpace(groupName) == "" {
		return fmt.Errorf("group name is required")
	}
	if len(targets) == 0 {
		return nil
	}

	bakeFile := filepath.Join(repoRoot, "docker-bake.hcl")
	if _, err := os.Stat(bakeFile); err != nil {
		return fmt.Errorf("bake file not found: %w", err)
	}

	tmpFile, err := writeBakeFile(groupName, targets)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpFile) }()

	builder := buildxBuilderName()
	args := buildBakeArgs(builder, bakeFile, tmpFile, targets, groupName, verbose)
	return withBuildLock(lockRoot, "bake", func() error {
		return runBakeCommand(ctx, runner, repoRoot, args, verbose)
	})
}

func buildxHint(output string) string {
	if output == "" {
		return ""
	}
	normalized := strings.ToLower(output)
	if !strings.Contains(normalized, "public.ecr.aws") {
		return ""
	}
	if strings.Contains(normalized, "403") || strings.Contains(normalized, "forbidden") || strings.Contains(normalized, "unauthorized") {
		return "Hint: public.ecr.aws denied the request. Docker credentials may be stale. Try 'docker logout public.ecr.aws' and retry, or run 'docker login public.ecr.aws'."
	}
	return ""
}

func buildBakeArgs(
	builder string,
	bakeFile string,
	tmpFile string,
	targets []bakeTarget,
	groupName string,
	verbose bool,
) []string {
	args := []string{"buildx", "bake", "--builder", builder}
	args = append(args, bakeAllowArgs(targets)...)
	args = append(args, bakeProvenanceArgs()...)
	args = append(args, "-f", bakeFile, "-f", tmpFile)
	if verbose {
		args = append(args, "--progress", "plain")
	}
	args = append(args, groupName)
	return args
}

func runBakeCommand(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	args []string,
	verbose bool,
) error {
	if verbose {
		return runner.Run(ctx, repoRoot, "docker", args...)
	}
	output, err := runner.RunOutput(ctx, repoRoot, "docker", args...)
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return err
	}
	if hint := buildxHint(trimmed); hint != "" {
		return fmt.Errorf("buildx bake failed: %w\n%s\n%s", err, trimmed, hint)
	}
	return fmt.Errorf("buildx bake failed: %w\n%s", err, trimmed)
}
