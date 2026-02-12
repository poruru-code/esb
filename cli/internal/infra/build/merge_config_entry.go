// Where: cli/internal/infra/build/merge_config_entry.go
// What: Entry point and orchestration for deploy config merge.
// Why: Keep merge sequence control separate from merge implementations.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

const deployLockTimeout = 30 * time.Second

// MergeConfig merges new configuration files into the existing CONFIG_DIR.
// It implements the "last-write-wins" merge strategy for multiple deployments.
func MergeConfig(outputDir, templatePath, composeProject, env string) error {
	configDir, err := staging.ConfigDir(templatePath, composeProject, env)
	if err != nil {
		return err
	}
	if err := ensureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	lockPath := filepath.Join(configDir, ".deploy.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	if err := acquireDeployLock(lockFile, deployLockTimeout); err != nil {
		return fmt.Errorf("failed to acquire deploy lock: %w", err)
	}

	srcConfigDir := filepath.Join(outputDir, "config")

	if err := mergeFunctionsYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge functions.yml: %w", err)
	}
	if err := mergeRoutingYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge routing.yml: %w", err)
	}
	if err := mergeResourcesYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge resources.yml: %w", err)
	}
	if err := mergeImageImportManifest(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge image-import.json: %w", err)
	}

	return nil
}
