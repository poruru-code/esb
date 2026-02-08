// Where: cli/internal/usecase/deploy/runtime_config.go
// What: Runtime config synchronization helpers for deploy workflow.
// Why: Isolate file/volume/container copy mechanics from orchestration logic.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

type runtimeConfigTarget struct {
	BindPath    string
	VolumeName  string
	ContainerID string
}

func (w Workflow) syncRuntimeConfig(req Request) error {
	composeProject := strings.TrimSpace(req.Context.ComposeProject)
	if composeProject == "" {
		return nil
	}
	stagingDir, err := staging.ConfigDir(req.TemplatePath, composeProject, req.Env)
	if err != nil {
		return err
	}
	if _, err := os.Stat(stagingDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staging dir: %w", err)
	}
	target, err := resolveRuntimeConfigTarget(composeProject)
	if err != nil {
		return err
	}
	return w.syncRuntimeConfigToTarget(stagingDir, target)
}

func (w Workflow) syncRuntimeConfigToTarget(stagingDir string, target runtimeConfigTarget) error {
	if target.BindPath == "" && target.VolumeName == "" && target.ContainerID == "" {
		return nil
	}
	if target.BindPath != "" {
		if samePath(target.BindPath, stagingDir) {
			return nil
		}
		return copyConfigFiles(stagingDir, target.BindPath)
	}
	var containerErr error
	if target.ContainerID != "" {
		copyErr := copyConfigToContainer(w.ComposeRunner, stagingDir, target.ContainerID)
		if copyErr == nil {
			return nil
		}
		containerErr = copyErr
	}
	if target.VolumeName != "" {
		volumeErr := copyConfigToVolume(w.ComposeRunner, stagingDir, target.VolumeName)
		if volumeErr == nil {
			return nil
		}
		if containerErr != nil {
			return fmt.Errorf("sync runtime config failed: %w", errors.Join(containerErr, volumeErr))
		}
		return fmt.Errorf("sync runtime config failed: %w", volumeErr)
	}
	return containerErr
}

func resolveRuntimeConfigTarget(composeProject string) (runtimeConfigTarget, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return runtimeConfigTarget{}, fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return runtimeConfigTarget{}, fmt.Errorf("list containers: %w", err)
	}
	var fallback *container.Summary
	for i, ctr := range containers {
		if ctr.Labels == nil {
			continue
		}
		if ctr.Labels[compose.ComposeServiceLabel] == "gateway" {
			fallback = &containers[i]
			break
		}
	}
	if fallback == nil && len(containers) > 0 {
		fallback = &containers[0]
	}
	if fallback == nil {
		return runtimeConfigTarget{}, nil
	}
	target := runtimeConfigTarget{ContainerID: fallback.ID}
	for _, mount := range fallback.Mounts {
		if mount.Destination != "/app/runtime-config" {
			continue
		}
		if strings.EqualFold(string(mount.Type), "bind") {
			target.BindPath = mount.Source
			return target, nil
		}
		if strings.EqualFold(string(mount.Type), "volume") {
			if mount.Name != "" {
				target.VolumeName = mount.Name
				return target, nil
			}
			if mount.Source != "" {
				target.BindPath = mount.Source
				return target, nil
			}
		}
	}
	return target, nil
}

func copyConfigFiles(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml", "image-import.json"} {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := filepath.Join(destDir, name)
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyConfigToVolume(runner compose.CommandRunner, srcDir, volume string) error {
	if runner == nil {
		return errComposeRunnerNotConfigured
	}
	cmd := "mkdir -p /app/runtime-config && " +
		"for f in functions.yml routing.yml resources.yml image-import.json; do " +
		"if [ -f \"/src/${f}\" ]; then cp -f \"/src/${f}\" \"/app/runtime-config/${f}\"; fi; " +
		"done"
	args := []string{
		"run",
		"--rm",
		"-v", volume + ":/app/runtime-config",
		"-v", srcDir + ":/src:ro",
		"alpine",
		"sh",
		"-c",
		cmd,
	}
	if err := runner.Run(context.Background(), "", "docker", args...); err != nil {
		return fmt.Errorf("copy config to volume: %w", err)
	}
	return nil
}

func copyConfigToContainer(runner compose.CommandRunner, srcDir, containerID string) error {
	if runner == nil {
		return errComposeRunnerNotConfigured
	}
	ctx := context.Background()
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml", "image-import.json"} {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := containerID + ":/app/runtime-config/" + name
		if err := runner.Run(ctx, "", "docker", "cp", src, dest); err != nil {
			return fmt.Errorf("copy config to container: %w", err)
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dest, err)
	}
	return nil
}

func samePath(left, right string) bool {
	l, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	r, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return filepath.Clean(l) == filepath.Clean(r)
}
