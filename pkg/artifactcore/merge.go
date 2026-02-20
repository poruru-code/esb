package artifactcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	mergeLockWaitTimeout  = 30 * time.Second
	mergeLockPollInterval = 50 * time.Millisecond
)

const mergeLockFileName = ".artifact-merge.lock"

func mergeWithManifest(manifestPath, outputDir string, manifest ArtifactManifest) error {
	if outputDir == "" {
		return fmt.Errorf("output dir is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	return withOutputDirLock(outputDir, func() error {
		for i := range manifest.Artifacts {
			runtimeDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, i)
			if err != nil {
				return err
			}
			if err := mergeOneRuntimeConfig(runtimeDir, outputDir); err != nil {
				return fmt.Errorf("merge artifacts[%d]: %w", i, err)
			}
		}
		return nil
	})
}

func withOutputDirLock(outputDir string, fn func() error) error {
	lockPath := filepath.Join(outputDir, mergeLockFileName)
	deadline := time.Now().Add(mergeLockWaitTimeout)
	for {
		lockFile, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			release := func() {
				_ = lockFile.Close()
				_ = os.Remove(lockPath)
			}
			if _, writeErr := fmt.Fprintf(lockFile, "%d\n", os.Getpid()); writeErr != nil {
				release()
				return fmt.Errorf("write merge lock file: %w", writeErr)
			}
			runErr := fn()
			release()
			return runErr
		}
		if !os.IsExist(err) {
			return fmt.Errorf("create merge lock file: %w", err)
		}
		recovered, recoverErr := tryRecoverStaleLock(lockPath)
		if recoverErr != nil {
			return recoverErr
		}
		if recovered {
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for merge lock: %s", lockPath)
		}
		time.Sleep(mergeLockPollInterval)
	}
}

func tryRecoverStaleLock(lockPath string) (bool, error) {
	ownerPID, hasPID, err := readLockOwnerPID(lockPath)
	if err != nil {
		return false, err
	}
	if !hasPID {
		return false, nil
	}
	alive, err := isProcessAlive(ownerPID)
	if err != nil {
		return false, err
	}
	if alive {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove stale merge lock file: %w", err)
	}
	return true, nil
}

func readLockOwnerPID(lockPath string) (int, bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read merge lock file: %w", err)
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if line == "" {
		return 0, false, nil
	}
	if strings.HasPrefix(line, "pid=") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "pid="))
	}
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		return 0, false, nil
	}
	return pid, true, nil
}

func isProcessAlive(pid int) (bool, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, fmt.Errorf("find lock owner process: %w", err)
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such process") || strings.Contains(msg, "process already finished") {
		return false, nil
	}
	if strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied") {
		return true, nil
	}
	return false, nil
}

func mergeOneRuntimeConfig(srcDir, destDir string) error {
	if err := mergeFunctionsYML(srcDir, destDir); err != nil {
		return err
	}
	if err := mergeRoutingYML(srcDir, destDir); err != nil {
		return err
	}
	if err := mergeResourcesYML(srcDir, destDir); err != nil {
		return err
	}
	return nil
}
