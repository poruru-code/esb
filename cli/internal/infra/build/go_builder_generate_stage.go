// Where: cli/internal/infra/build/go_builder_generate_stage.go
// What: Config generation and staging helpers for GoBuilder.
// Why: Keep generation-specific details out of Build orchestration.
package build

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	templategen "github.com/poruru/edge-serverless-box/cli/internal/infra/templategen"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func defaultGeneratorParameters() map[string]string {
	return map[string]string{
		"S3_ENDPOINT_HOST":       "s3-storage",
		"DYNAMODB_ENDPOINT_HOST": "database",
	}
}

func toAnyMap(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (b *GoBuilder) generateAndStageConfig(
	cfg config.GeneratorConfig,
	opts templategen.GenerateOptions,
	repoRoot string,
	templatePath string,
	composeProject string,
	env string,
	skipStaging bool,
) ([]template.FunctionSpec, error) {
	functions, err := b.Generate(cfg, opts)
	if err != nil {
		return nil, err
	}
	if skipStaging {
		return functions, nil
	}
	if err := stageConfigFiles(cfg.Paths.OutputDir, repoRoot, templatePath, composeProject, env); err != nil {
		return nil, err
	}
	return functions, nil
}

func stageConfigFiles(outputDir, repoRoot, templatePath, composeProject, env string) error {
	configDir := filepath.Join(outputDir, "config")
	stagingRoot, err := staging.BaseDir(templatePath, composeProject, env)
	if err != nil {
		return err
	}

	// Verify source config files exist.
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml"} {
		src := filepath.Join(configDir, name)
		if !fileExists(src) {
			return fmt.Errorf("config not found: %s", src)
		}
	}

	// Merge config files into CONFIG_DIR (with locking and atomic updates).
	if err := MergeConfig(outputDir, templatePath, composeProject, env); err != nil {
		return err
	}

	// Stage pyproject.toml for isolated builds.
	rootPyProject := filepath.Join(repoRoot, "pyproject.toml")
	if fileExists(rootPyProject) {
		destPyProject := filepath.Join(stagingRoot, "pyproject.toml")
		if err := copyFile(rootPyProject, destPyProject); err != nil {
			return err
		}
	}

	// Stage services/common and services/gateway for standardized structure.
	commonSrc := filepath.Join(repoRoot, "services", "common")
	commonDest := filepath.Join(stagingRoot, "services", "common")
	gatewaySrc := filepath.Join(repoRoot, "services", "gateway")
	gatewayDest := filepath.Join(stagingRoot, "services", "gateway")

	// Clean staging services dir.
	if err := removeDir(filepath.Join(stagingRoot, "services")); err != nil {
		return err
	}

	// Copy common.
	if err := copyDir(commonSrc, commonDest); err != nil {
		return err
	}

	// Copy gateway source (excluding staging dir to avoid infinite recursion).
	entries, err := os.ReadDir(gatewaySrc)
	if err != nil {
		return err
	}
	if err := ensureDir(gatewayDest); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == meta.StagingDir {
			continue
		}
		srcPath := filepath.Join(gatewaySrc, entry.Name())
		dstPath := filepath.Join(gatewayDest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
