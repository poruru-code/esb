package deployops

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const runtimeConfigSyncMountPath = "/runtime-config"

var runDockerForSync = runDockerCommand

func syncRuntimeConfig(stagingDir string, target RuntimeConfigTarget) error {
	stagingDir = strings.TrimSpace(stagingDir)
	target = target.normalized()
	if stagingDir == "" {
		return fmt.Errorf("staging runtime-config directory is required")
	}
	if target.isEmpty() {
		return errRuntimeConfigTargetRequired
	}
	if target.BindPath != "" {
		return syncRuntimeConfigToBindPath(stagingDir, target.BindPath)
	}
	return syncRuntimeConfigToVolume(stagingDir, target.VolumeName)
}

func syncRuntimeConfigToBindPath(stagingDir, runtimeConfigDir string) error {
	runtimeConfigDir = strings.TrimSpace(runtimeConfigDir)
	if runtimeConfigDir == "" {
		return errRuntimeConfigTargetRequired
	}
	if err := clearDirectory(runtimeConfigDir); err != nil {
		return err
	}
	if err := copyDirectory(stagingDir, runtimeConfigDir); err != nil {
		return err
	}
	return nil
}

func syncRuntimeConfigToVolume(stagingDir, volumeName string) error {
	volumeName = strings.TrimSpace(volumeName)
	if volumeName == "" {
		return errRuntimeConfigTargetRequired
	}
	absoluteStagingDir, err := filepath.Abs(stagingDir)
	if err != nil {
		return fmt.Errorf("resolve staging runtime-config directory: %w", err)
	}
	script := strings.Join([]string{
		"set -eu",
		"mkdir -p " + runtimeConfigSyncMountPath,
		"rm -rf " + runtimeConfigSyncMountPath + "/* " +
			runtimeConfigSyncMountPath + "/.[!.]* " +
			runtimeConfigSyncMountPath + "/..?* 2>/dev/null || true",
		"cp -a /src/. " + runtimeConfigSyncMountPath + "/",
	}, " && ")
	_, err = runDockerForSync(
		"run",
		"--rm",
		"-v", fmt.Sprintf("%s:%s", volumeName, runtimeConfigSyncMountPath),
		"-v", fmt.Sprintf("%s:/src:ro", absoluteStagingDir),
		"alpine:3.20",
		"sh",
		"-c",
		script,
	)
	if err != nil {
		return fmt.Errorf("sync runtime-config volume %s: %w", volumeName, err)
	}
	return nil
}

func clearDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare runtime-config directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read runtime-config directory %s: %w", dir, err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("clear runtime-config entry %s: %w", path, err)
		}
	}
	return nil
}

func copyDirectory(srcDir, destDir string) error {
	if _, err := os.Stat(srcDir); err != nil {
		return fmt.Errorf("staging runtime-config directory does not exist: %w", err)
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("calculate relative path for %s: %w", path, err)
		}
		if relativePath == "." {
			return nil
		}
		targetPath := filepath.Join(destDir, relativePath)
		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create runtime-config directory %s: %w", targetPath, err)
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime-config sync does not support symlink: %s", path)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("runtime-config sync supports only regular files: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read staging runtime-config file %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("create runtime-config parent directory %s: %w", targetPath, err)
		}
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("write runtime-config file %s: %w", targetPath, err)
		}
		return nil
	})
}
