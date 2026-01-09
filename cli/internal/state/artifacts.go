// Where: cli/internal/state/artifacts.go
// What: Build artifact detection helpers for state resolution.
// Why: Determine whether output directories indicate a successful build.
package state

import (
	"os"
	"path/filepath"
)

// HasBuildArtifacts reports whether the output environment directory contains
// the expected build artifacts for a built state.
func HasBuildArtifacts(outputEnvDir string) (bool, error) {
	functionsYml := filepath.Join(outputEnvDir, "config", "functions.yml")
	routingYml := filepath.Join(outputEnvDir, "config", "routing.yml")

	if !isFile(functionsYml) || !isFile(routingYml) {
		return false, nil
	}

	hasDockerfile, err := hasDockerfile(filepath.Join(outputEnvDir, "functions"))
	if err != nil {
		return false, err
	}
	return hasDockerfile, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func hasDockerfile(root string) (bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}

	found := false
	err = filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "Dockerfile" {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}
