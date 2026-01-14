// Where: cli/internal/generator/parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package generator

import (
	"encoding/json"

	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
	"sigs.k8s.io/yaml"
)

type ParseResult struct {
	Functions []FunctionSpec
	Resources ResourcesSpec
}

type FunctionSpec struct {
	LogicalID               string
	Name                    string
	CodeURI                 string
	Handler                 string
	Runtime                 string
	Timeout                 int
	MemorySize              int
	HasRequirements         bool
	Environment             map[string]string
	Events                  []EventSpec
	Scaling                 ScalingSpec
	Layers                  []LayerSpec
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

type LayerSpec struct {
	Name                    string
	ContentURI              string
	CompatibleArchitectures []string
}

type RuntimeManagementConfig struct {
	UpdateRuntimeOn string
}

type DynamoDBSpec struct {
	TableName              string
	KeySchema              any
	AttributeDefinitions   any
	GlobalSecondaryIndexes any
	BillingMode            string
	ProvisionedThroughput  any
}

type S3Spec struct {
	BucketName string
}

type ResourcesSpec struct {
	DynamoDB []DynamoDBSpec
	S3       []S3Spec
	Layers   []LayerSpec
}

func ParseSAMTemplate(content string, parameters map[string]string) (ParseResult, error) {
	if parameters == nil {
		parameters = map[string]string{}
	}

	jsonData, err := yaml.YAMLToJSON([]byte(content))
	if err != nil {
		return ParseResult{}, err
	}

	// Decode into generated SAM model
	var template schema.SamModel
	if err := json.Unmarshal(jsonData, &template); err != nil {
		return ParseResult{}, err
	}

	data, err := decodeYAML(content)
	if err != nil {
		return ParseResult{}, err
	}

	if asMap(data["Resources"]) == nil {
		return ParseResult{}, nil
	}

	functionGlobals := extractFunctionGlobals(data)
	defaults := parseFunctionDefaults(functionGlobals, parameters)

	layerMap, layers := parseLayerResources(template.Resources, parameters)
	parsedResources := parseOtherResources(template.Resources, parameters)
	parsedResources.Layers = layers

	functions := parseFunctions(template.Resources, defaults, layerMap, parameters)

	return ParseResult{Functions: functions, Resources: parsedResources}, nil
}
