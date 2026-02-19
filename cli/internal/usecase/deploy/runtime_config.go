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
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

type runtimeConfigTarget struct {
	BindPath    string
	VolumeName  string
	ContainerID string
}

const runtimeConfigMountPath = constants.RuntimeConfigMountPath

var runtimeConfigFiles = []string{
	"functions.yml",
	"routing.yml",
	"resources.yml",
}

func (w Workflow) syncRuntimeConfigFromDir(composeProject, stagingDir string) error {
	if strings.TrimSpace(composeProject) == "" {
		return nil
	}
	if strings.TrimSpace(stagingDir) == "" {
		return fmt.Errorf("staging dir is required")
	}
	if _, err := os.Stat(stagingDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staging dir: %w", err)
	}
	target, err := w.resolveRuntimeConfigTarget(composeProject)
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

func (w Workflow) resolveRuntimeConfigTarget(composeProject string) (runtimeConfigTarget, error) {
	if w.DockerClient == nil {
		return runtimeConfigTarget{}, nil
	}
	client, err := w.newDockerClient()
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
	for _, selected := range orderedRuntimeConfigContainers(containers) {
		target := runtimeConfigTarget{ContainerID: selected.ID}
		mountFound := false
		for _, mount := range selected.Mounts {
			if mount.Destination != runtimeConfigMountPath {
				continue
			}
			mountFound = true
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
		if mountFound {
			// Destination exists but metadata is incomplete; fall back to container copy.
			return target, nil
		}
	}
	return runtimeConfigTarget{}, nil
}

func orderedRuntimeConfigContainers(containers []container.Summary) []container.Summary {
	if len(containers) == 0 {
		return nil
	}
	copied := append([]container.Summary(nil), containers...)
	sort.SliceStable(copied, func(i, j int) bool {
		iPriority := runtimeConfigServicePriority(copied[i])
		jPriority := runtimeConfigServicePriority(copied[j])
		if iPriority != jPriority {
			return iPriority < jPriority
		}
		iRunning := strings.EqualFold(copied[i].State, "running")
		jRunning := strings.EqualFold(copied[j].State, "running")
		if iRunning != jRunning {
			return iRunning
		}
		return gatewayContainerName(copied[i]) < gatewayContainerName(copied[j])
	})
	return copied
}

func runtimeConfigServicePriority(summary container.Summary) int {
	service := strings.TrimSpace(summary.Labels[compose.ComposeServiceLabel])
	if service == "gateway" {
		return 0
	}
	return 1
}

func copyConfigFiles(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	for _, name := range runtimeConfigFiles {
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
	cmd := "mkdir -p " + runtimeConfigMountPath + " && " +
		"for f in " + strings.Join(runtimeConfigFiles, " ") + "; do " +
		"if [ -f \"/src/${f}\" ]; then cp -f \"/src/${f}\" \"" + runtimeConfigMountPath + "/${f}\"; fi; " +
		"done"
	args := []string{
		"run",
		"--rm",
		"-v", volume + ":" + runtimeConfigMountPath,
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
	for _, name := range runtimeConfigFiles {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := containerID + ":" + runtimeConfigMountPath + "/" + name
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
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", destDir, err)
	}
	out, err := os.CreateTemp(destDir, "."+filepath.Base(dest)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	tempPath := out.Name()
	keepTemp := true
	closed := false
	defer func() {
		if !closed {
			_ = out.Close()
		}
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dest, err)
	}
	closed = true
	if err := os.Rename(tempPath, dest); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tempPath, dest, err)
	}
	keepTemp = false
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
