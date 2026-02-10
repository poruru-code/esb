// Where: cli/internal/infra/build/merge_config_lock_io.go
// What: deploy lock and atomic file IO helpers for merge config.
// Why: Centralize locking and persistence concerns for safe config updates.
package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

func acquireDeployLock(lockFile *os.File, timeout time.Duration) error {
	if lockFile == nil {
		return fmt.Errorf("lock file is nil")
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout after %s", timeout)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
}

// loadYamlFile loads a YAML file into a map.
func loadYamlFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func loadImageImportManifest(path string) (imageImportManifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return imageImportManifest{}, false, nil
		}
		return imageImportManifest{}, false, err
	}
	var result imageImportManifest
	if err := json.Unmarshal(data, &result); err != nil {
		return imageImportManifest{}, false, err
	}
	if result.Images == nil {
		result.Images = []imageImportEntry{}
	}
	return result, true, nil
}

// atomicWriteYaml writes data to a YAML file atomically using tmp + rename.
func atomicWriteYaml(path string, data map[string]any) error {
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = f.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func atomicWriteJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = f.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
