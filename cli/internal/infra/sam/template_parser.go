// Where: cli/internal/infra/sam/template_parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package sam

import (
	"fmt"

	"github.com/poruru-code/esb/cli/internal/domain/template"
	"github.com/poruru-code/esb/cli/internal/domain/value"
)

func ParseSAMTemplate(content string, parameters map[string]string) (template.ParseResult, error) {
	if parameters == nil {
		parameters = map[string]string{}
	}

	data, err := DecodeYAML(content)
	if err != nil {
		return template.ParseResult{}, err
	}
	mergedParams := extractParameterDefaults(data)
	if mergedParams == nil {
		mergedParams = map[string]string{}
	}
	for k, v := range parameters {
		mergedParams[k] = v
	}

	resolver := NewIntrinsicResolver(mergedParams)
	resolver.RawConditions = value.AsMap(data["Conditions"])

	resolvedAny, err := ResolveAll(
		&Context{MaxDepth: maxResolveDepth},
		data,
		resolver,
	)
	if err != nil {
		return template.ParseResult{}, err
	}
	resolved := value.AsMap(resolvedAny)
	if resolved == nil {
		return template.ParseResult{}, fmt.Errorf("unexpected yaml root")
	}

	model, err := DecodeTemplate(resolved)
	if err != nil {
		return template.ParseResult{}, err
	}

	if value.AsMap(resolved["Resources"]) == nil {
		return template.ParseResult{}, nil
	}

	functionGlobals := extractFunctionGlobals(resolved)
	defaults := parseFunctionDefaults(functionGlobals)
	warnings := &warningCollector{}

	layerMap, layers := parseLayerResources(model.Resources)
	parsedResources := parseOtherResources(model.Resources, warnings.warnf)
	parsedResources.Layers = layers

	functions, err := parseFunctions(model.Resources, defaults, layerMap, warnings.warnf)
	if err != nil {
		return template.ParseResult{}, err
	}

	return template.ParseResult{
		Functions: functions,
		Resources: parsedResources,
		Warnings:  warnings.list(),
	}, nil
}

func extractParameterDefaults(data map[string]any) map[string]string {
	params := value.AsMap(data["Parameters"])
	if params == nil {
		return nil
	}
	defaults := map[string]string{}
	for name, val := range params {
		m := value.AsMap(val)
		if m == nil {
			continue
		}
		if def := m["Default"]; def != nil {
			defaults[name] = fmt.Sprint(def)
		}
	}
	return defaults
}
