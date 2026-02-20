// Where: cli/internal/infra/sam/template_functions_parse.go
// What: Entry point for SAM function resource extraction.
// Why: Keep top-level parse dispatch separate from per-resource implementations.
package sam

import (
	"github.com/poruru-code/esb/cli/internal/domain/manifest"
	"github.com/poruru-code/esb/cli/internal/domain/template"
	"github.com/poruru-code/esb/cli/internal/domain/value"
)

func parseFunctions(
	resources map[string]any,
	defaults functionDefaults,
	layerMap map[string]manifest.LayerSpec,
	warnf func(string, ...any),
) ([]template.FunctionSpec, error) {
	functions := make([]template.FunctionSpec, 0)
	for _, logicalID := range sortedMapKeys(resources) {
		raw := resources[logicalID]
		m := value.AsMap(raw)
		if m == nil {
			continue
		}
		resourceType := value.AsString(m["Type"])
		switch resourceType {
		case "AWS::Serverless::Function":
			fn, ok, err := parseServerlessFunction(logicalID, m, defaults, layerMap, warnf)
			if err != nil {
				return nil, err
			}
			if ok {
				functions = append(functions, fn)
			}
		case "AWS::Lambda::Function":
			fn, ok, err := parseLambdaFunction(logicalID, m, defaults, warnf)
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
