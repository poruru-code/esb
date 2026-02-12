// Where: cli/internal/infra/build/go_builder_lock.go
// What: File lock helpers used by build and bake operations.
// Why: Keep cross-process lock handling centralized and reusable.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func withBuildLock(lockRoot, name string, fn func() error) error {
	key := strings.TrimSpace(name)
	if key == "" {
		return fn()
	}
	if strings.TrimSpace(lockRoot) == "" {
		return fmt.Errorf("lock root is required")
	}
	if err := ensureDir(lockRoot); err != nil {
		return err
	}
	lockPath := filepath.Join(lockRoot, fmt.Sprintf(".lock-%s", key))
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}
