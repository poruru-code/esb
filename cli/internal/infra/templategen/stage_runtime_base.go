// Where: cli/internal/infra/templategen/stage_runtime_base.go
// What: Staging helpers for runtime base image build context.
// Why: Keep artifact outputs self-contained for non-CLI image preparation.
package templategen

import (
	"fmt"
	"path/filepath"
)

const (
	runtimeBaseContextDirName       = "runtime-base"
	runtimeBasePythonSourceRelDir   = "runtime-hooks/python"
	runtimeBasePythonDockerfileRel  = "runtime-hooks/python/docker/Dockerfile"
	runtimeBaseJavaAgentSourceRel   = "runtime-hooks/java/agent/lambda-java-agent.jar"
	runtimeBaseJavaWrapperSourceRel = "runtime-hooks/java/wrapper/lambda-java-wrapper.jar"
	runtimeBaseTemplatesSourceRel   = "cli/assets/runtime-templates"
	runtimeBaseTemplatesRelDir      = "runtime-templates"
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
	if _, err := stageRuntimeBaseFileIfExists(
		ctx,
		runtimeBaseJavaAgentSourceRel,
		runtimeBaseJavaAgentSourceRel,
		"java runtime agent",
	); err != nil {
		return err
	}
	if _, err := stageRuntimeBaseFileIfExists(
		ctx,
		runtimeBaseJavaWrapperSourceRel,
		runtimeBaseJavaWrapperSourceRel,
		"java runtime wrapper",
	); err != nil {
		return err
	}
	templateSource := filepath.Join(ctx.ProjectRoot, runtimeBaseTemplatesSourceRel)
	if !dirExists(templateSource) {
		return fmt.Errorf("runtime templates source not found: %s", templateSource)
	}
	templateDest := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBaseTemplatesRelDir)
	if err := copyDir(templateSource, templateDest); err != nil {
		return fmt.Errorf("stage runtime templates: %w", err)
	}
	dockerfilePath := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBasePythonDockerfileRel)
	if !fileExists(dockerfilePath) {
		return fmt.Errorf("python runtime base dockerfile not found after staging: %s", dockerfilePath)
	}
	if !dirExists(filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBaseTemplatesRelDir)) {
		return fmt.Errorf("runtime templates directory not found after staging: %s", filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, runtimeBaseTemplatesRelDir))
	}
	ctx.verbosef("Staged runtime base context: %s\n", filepath.Join(ctx.OutputDir, runtimeBaseContextDirName))
	return nil
}

func stageRuntimeBaseFileIfExists(ctx stageContext, sourceRel, destRel, label string) (bool, error) {
	source := filepath.Join(ctx.ProjectRoot, sourceRel)
	if !fileExists(source) {
		ctx.verbosef("Skipping %s staging; source not found: %s\n", label, source)
		return false, nil
	}
	dest := filepath.Join(ctx.OutputDir, runtimeBaseContextDirName, destRel)
	if err := copyFile(source, dest); err != nil {
		return false, fmt.Errorf("stage %s: %w", label, err)
	}
	return true, nil
}
