// Where: cli/internal/infra/sam/template_functions_parse.go
// What: Entry point for SAM function resource extraction.
// Why: Keep top-level parse dispatch separate from per-resource implementations.
package sam

import (
	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

func parseFunctions(
	resources map[string]any,
	defaults functionDefaults,
	layerMap map[string]manifest.LayerSpec,
) ([]template.FunctionSpec, error) {
	functions := make([]template.FunctionSpec, 0)
	for logicalID, raw := range resources {
		m := value.AsMap(raw)
		if m == nil {
			continue
		}
		resourceType := value.AsString(m["Type"])
		switch resourceType {
		case "AWS::Serverless::Function":
			fn, ok, err := parseServerlessFunction(logicalID, m, defaults, layerMap)
			if err != nil {
				return nil, err
			}
			if ok {
				functions = append(functions, fn)
			}
		case "AWS::Lambda::Function":
			fn, ok, err := parseLambdaFunction(logicalID, m, defaults)
			if err != nil {
				return nil, err
			}
			if ok {
				functions = append(functions, fn)
			}
		}
	}

	return functions, nil
}
