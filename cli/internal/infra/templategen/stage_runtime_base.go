// Where: cli/internal/infra/templategen/stage_runtime_base.go
// What: Staging helpers for runtime base image build context.
// Why: Keep artifact outputs self-contained for non-CLI image preparation.
package templategen

import (
	"fmt"
	"path/filepath"
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
	if err := copyDir(sourceDir, destDir); err != nil {
		return fmt.Errorf("stage python runtime base context: %w", err)
	}
	dockerfilePath := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBasePythonDockerfileRel)
	if !fileExists(dockerfilePath) {
		return fmt.Errorf("python runtime base dockerfile not found after staging: %s", dockerfilePath)
	}
	ctx.verbosef("Staged runtime base context: %s\n", filepath.Join(ctx.OutputDir, runtimeBaseContextDirName))
	return nil
}
