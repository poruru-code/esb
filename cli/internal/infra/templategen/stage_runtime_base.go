// Where: cli/internal/infra/templategen/stage_runtime_base.go
// What: Staging helpers for runtime base image build context.
// Why: Keep artifact outputs self-contained for non-CLI image preparation.
package templategen

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

const (
	runtimeBaseContextDirName      = "runtime-base"
	runtimeBasePythonSourceRelDir  = "runtime-hooks/python"
	runtimeBasePythonDockerfileRel = "runtime-hooks/python/docker/Dockerfile"
)

func stageRuntimeBaseContext(ctx stageContext) error {
	if ctx.DryRun {
		return nil
	}
	sourceDir := filepath.Join(ctx.ProjectRoot, runtimeBasePythonSourceRelDir)
	if !dirExists(sourceDir) {
		return fmt.Errorf("python runtime base source not found: %s", sourceDir)
	}
	destDir := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBasePythonSourceRelDir)
	if err := copyRuntimeBaseDir(sourceDir, destDir); err != nil {
		return fmt.Errorf("stage python runtime base context: %w", err)
	}
	dockerfilePath := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBasePythonDockerfileRel)
	if !fileExists(dockerfilePath) {
		return fmt.Errorf("python runtime base dockerfile not found after staging: %s", dockerfilePath)
	}
	ctx.verbosef("Staged runtime base context: %s\n", filepath.Join(ctx.OutputDir, runtimeBaseContextDirName))
	return nil
}

func copyRuntimeBaseDir(src, dst string) error {
	if err := ensureDir(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(current string, entryDir fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entryDir.IsDir() {
			if shouldSkipRuntimeBaseDir(rel, entryDir.Name()) {
				return filepath.SkipDir
			}
			return ensureDir(filepath.Join(dst, rel))
		}
		if shouldSkipRuntimeBaseFile(rel, entryDir.Name()) {
			return nil
		}
		return copyFile(current, filepath.Join(dst, rel))
	})
}

func shouldSkipRuntimeBaseDir(rel, name string) bool {
	if rel == "." {
		return false
	}
	if name == "__pycache__" {
		return true
	}
	if name == "tests" {
		return true
	}
	return false
}

func shouldSkipRuntimeBaseFile(rel, name string) bool {
	if strings.HasSuffix(name, ".pyc") {
		return true
	}
	normalized := strings.TrimPrefix(rel, "./")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "__pycache__" || segment == "tests" {
			return true
		}
	}
	// Keep compatibility with paths that can contain repeated slashes.
	cleaned := path.Clean("/" + normalized)
	return strings.Contains(cleaned, "/__pycache__/") || strings.Contains(cleaned, "/tests/")
}
