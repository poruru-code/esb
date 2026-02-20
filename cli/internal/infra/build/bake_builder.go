// Where: cli/internal/infra/build/bake_builder.go
// What: Buildx builder lifecycle and validation helpers.
// Why: Keep builder provisioning logic separate from bake execution flow.
package build

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

type buildxBuilderOptions struct {
	NetworkMode string
	ConfigPath  string
}

func ensureBuildxBuilder(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	lockRoot string,
	opts buildxBuilderOptions,
) error {
	if runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("repo root is required")
	}
	builder := buildxBuilderName()
	needsRecreate := false
	desiredProxyEnv := buildxProxyDriverEnvMap()
	return withBuildLock(lockRoot, "buildx", func() error {
		output, err := runner.RunOutput(
			ctx,
			root,
			"docker",
			"buildx",
			"inspect",
			"--builder",
			builder,
		)
		if err == nil && strings.TrimSpace(opts.NetworkMode) != "" {
			mode, modeErr := buildxBuilderNetworkMode(ctx, runner, root, builder)
			if modeErr != nil || mode != opts.NetworkMode {
				needsRecreate = true
			}
		}
		if err == nil {
			proxyMismatch, proxyErr := buildxBuilderProxyMismatch(
				ctx,
				runner,
				root,
				builder,
				desiredProxyEnv,
			)
			if proxyErr != nil || proxyMismatch {
				needsRecreate = true
			}
		}
		if err != nil || needsRecreate {
			if needsRecreate {
				_ = runner.Run(ctx, root, "docker", "buildx", "rm", builder)
			}
			createArgs := []string{
				"buildx",
				"create",
				"--name",
				builder,
				"--driver",
				"docker-container",
				"--use",
				"--bootstrap",
			}
			if strings.TrimSpace(opts.NetworkMode) != "" {
				createArgs = append(createArgs, "--driver-opt", fmt.Sprintf("network=%s", opts.NetworkMode))
			}
			if proxyOpts := buildxProxyDriverOptsFromMap(desiredProxyEnv); len(proxyOpts) > 0 {
				for _, opt := range proxyOpts {
					createArgs = append(createArgs, "--driver-opt", opt)
				}
			}
			if configPath := strings.TrimSpace(opts.ConfigPath); configPath != "" {
				if info, statErr := os.Stat(configPath); statErr == nil && !info.IsDir() {
					createArgs = append(createArgs, "--buildkitd-config", configPath)
				}
			}
			createOutput, createErr := runner.RunOutput(ctx, root, "docker", createArgs...)
			if createErr != nil {
				lower := strings.ToLower(string(createOutput))
				if strings.Contains(lower, "existing instance") || strings.Contains(lower, "already exists") {
					if strings.TrimSpace(opts.NetworkMode) != "" {
						if mode, modeErr := buildxBuilderNetworkMode(ctx, runner, root, builder); modeErr == nil && mode != opts.NetworkMode {
							return fmt.Errorf(
								"buildx builder %s uses network mode %s (expected %s)",
								builder,
								mode,
								opts.NetworkMode,
							)
						}
					}
					if err := runner.Run(ctx, root, "docker", "buildx", "use", builder); err != nil {
						return err
					}
				} else {
					return createErr
				}
			}
			output, err = runner.RunOutput(
				ctx,
				root,
				"docker",
				"buildx",
				"inspect",
				"--builder",
				builder,
				"--bootstrap",
			)
			if err != nil {
				return err
			}
		}
		driver := parseBuildxDriver(output)
		if driver == "" {
			return fmt.Errorf("buildx builder %s has no driver info", builder)
		}
		if !strings.EqualFold(driver, "docker-container") {
			return fmt.Errorf("buildx builder %s uses driver %s (expected docker-container)", builder, driver)
		}
		return nil
	})
}

func buildxBuilderNetworkMode(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	builder string,
) (string, error) {
	containerName := fmt.Sprintf("buildx_buildkit_%s0", builder)
	output, err := runner.RunOutput(
		ctx,
		repoRoot,
		"docker",
		"inspect",
		"-f",
		"{{.HostConfig.NetworkMode}}",
		containerName,
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func parseBuildxDriver(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Driver:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Driver:"))
		}
	}
	return ""
}

func mergeStringMap(base, extra map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}
