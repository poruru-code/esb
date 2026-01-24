// Where: cli/internal/generator/parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package generator

import (
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/sam"
)

type ParseResult struct {
	Functions []FunctionSpec
	Resources manifest.ResourcesSpec
}

type FunctionSpec struct {
	LogicalID               string
	Name                    string
	ImageName               string
	CodeURI                 string
	Handler                 string
	Runtime                 string
	Timeout                 int
	MemorySize              int
	HasRequirements         bool
	Environment             map[string]string
	Events                  []EventSpec
	Scaling                 ScalingSpec
	Layers                  []manifest.LayerSpec
	Architectures           []string
	RuntimeManagementConfig RuntimeManagementConfig
}

type EventSpec struct {
	Type               string
	Path               string
	Method             string
	ScheduleExpression string
	Input              string
}

type ScalingSpec struct {
	MaxCapacity *int
	MinCapacity *int
}

// Moved to manifest package

type RuntimeManagementConfig struct {
	UpdateRuntimeOn string
}

func ParseSAMTemplate(content string, parameters map[string]string) (ParseResult, error) {
	if parameters == nil {
		parameters = map[string]string{}
	}

	data, err := sam.DecodeYAML(content)
	if err != nil {
		return ParseResult{}, err
	}
	mergedParams := extractParameterDefaults(data)
	if mergedParams == nil {
		mergedParams = map[string]string{}
	}
	for k, v := range parameters {
		mergedParams[k] = v
	}

	resolver := NewIntrinsicResolver(mergedParams)
	resolver.RawConditions = asMap(data["Conditions"])

	resolvedAny, err := sam.ResolveAll(
		&sam.Context{MaxDepth: maxResolveDepth},
		data,
		resolver,
	)
	if err != nil {
		return ParseResult{}, err
	}
	resolved := asMap(resolvedAny)
	if resolved == nil {
		return ParseResult{}, fmt.Errorf("unexpected yaml root")
	}

	template, err := sam.DecodeTemplate(resolved)
	if err != nil {
		return ParseResult{}, err
	}

	if asMap(resolved["Resources"]) == nil {
		return ParseResult{}, nil
	}

	functionGlobals := extractFunctionGlobals(resolved)
	defaults := parseFunctionDefaults(functionGlobals)

	layerMap, layers := parseLayerResources(template.Resources)
	parsedResources := parseOtherResources(template.Resources)
	parsedResources.Layers = layers

	functions := parseFunctions(template.Resources, defaults, layerMap)

	return ParseResult{Functions: functions, Resources: parsedResources}, nil
}

func extractParameterDefaults(data map[string]any) map[string]string {
	params := asMap(data["Parameters"])
	if params == nil {
		return nil
	}
	defaults := map[string]string{}
	for name, val := range params {
		m := asMap(val)
		if m == nil {
			continue
		}
		if def := m["Default"]; def != nil {
			defaults[name] = fmt.Sprint(def)
		}
	}
	return defaults
}
