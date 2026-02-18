// Where: cli/internal/infra/build/go_builder_generate_stage.go
// What: Config generation and staging helpers for GoBuilder.
// Why: Keep generation-specific details out of Build orchestration.
package build

import (
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	templategen "github.com/poruru/edge-serverless-box/cli/internal/infra/templategen"
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
) ([]template.FunctionSpec, error) {
	return b.Generate(cfg, opts)
}
