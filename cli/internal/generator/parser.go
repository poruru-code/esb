// Where: cli/internal/generator/parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package generator

import (
	"encoding/json"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
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
	Path   string
	Method string
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

	jsonData, err := validateSAMTemplate([]byte(content))
	if err != nil {
		return ParseResult{}, err
	}

	var template schema.SamSchemaJson
	if err := json.Unmarshal(jsonData, &template); err != nil {
		return ParseResult{}, err
	}

	data, err := decodeYAML(content)
	if err != nil {
		return ParseResult{}, err
	}

	resources := asMap(data["Resources"])
	if resources == nil {
		return ParseResult{}, nil
	}

	globals := template.Globals
	functionGlobals := &schema.SamSchemaJsonGlobalsFunction{}
	if globals != nil && globals.Function != nil {
		functionGlobals = globals.Function
	}

	defaultRuntime := derefString(functionGlobals.Runtime, "python3.12")
	defaultHandler := derefString(functionGlobals.Handler, "lambda_function.lambda_handler")
	defaultTimeout := derefInt(functionGlobals.Timeout, 30)
	defaultMemory := derefInt(functionGlobals.MemorySize, 128)
	defaultLayers := functionGlobals.Layers
	defaultArchitectures := copyStringSlice(functionGlobals.Architectures)
	defaultRuntimeManagement := runtimeManagementFromGlobals(functionGlobals.RuntimeManagementConfig)
	defaultEnv := map[string]string{}
	if functionGlobals.Environment != nil {
		for key, value := range functionGlobals.Environment.Variables {
			defaultEnv[key] = resolveIntrinsic(asString(value), parameters)
		}
	}

	parsedResources := ResourcesSpec{}
	layerMap := map[string]LayerSpec{}
	for logicalID, resource := range template.Resources {
		if resource.Type != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := resource.Properties
		if props == nil {
			continue
		}
		layerName := derefString(props.LayerName, logicalID)
		layerName = resolveIntrinsic(layerName, parameters)
		contentURI := derefString(props.ContentUri, "./")
		contentURI = resolveIntrinsic(contentURI, parameters)
		contentURI = ensureTrailingSlash(contentURI)
		spec := LayerSpec{
			Name:                    layerName,
			ContentURI:              contentURI,
			CompatibleArchitectures: copyStringSlice(props.CompatibleArchitectures),
		}
		layerMap[logicalID] = spec
		parsedResources.Layers = append(parsedResources.Layers, spec)
	}

	for logicalID, value := range resources {
		resource := asMap(value)
		if resource == nil {
			continue
		}
		resourceType := asString(resource["Type"])
		if resourceType == "AWS::Serverless::LayerVersion" || resourceType == "AWS::Serverless::Function" {
			continue
		}
		props := asMap(resource["Properties"])

		switch resourceType {
		case "AWS::DynamoDB::Table":
			tableName := asString(props["TableName"])
			if tableName == "" {
				tableName = logicalID
			}
			tableName = resolveIntrinsic(tableName, parameters)
			parsedResources.DynamoDB = append(parsedResources.DynamoDB, DynamoDBSpec{
				TableName:              tableName,
				KeySchema:              props["KeySchema"],
				AttributeDefinitions:   props["AttributeDefinitions"],
				GlobalSecondaryIndexes: props["GlobalSecondaryIndexes"],
				BillingMode:            asStringDefault(props["BillingMode"], "PROVISIONED"),
				ProvisionedThroughput:  props["ProvisionedThroughput"],
			})
		case "AWS::S3::Bucket":
			bucketName := asString(props["BucketName"])
			if bucketName == "" {
				bucketName = strings.ToLower(logicalID)
			}
			bucketName = resolveIntrinsic(bucketName, parameters)
			parsedResources.S3 = append(parsedResources.S3, S3Spec{BucketName: bucketName})
		}
	}

	functions := make([]FunctionSpec, 0, len(template.Resources))
	for logicalID, resource := range template.Resources {
		if resource.Type != "AWS::Serverless::Function" {
			continue
		}
		props := resource.Properties
		if props == nil {
			continue
		}

		fnName := derefString(props.FunctionName, logicalID)
		fnName = resolveIntrinsic(fnName, parameters)
		codeURI := derefString(props.CodeUri, "./")
		codeURI = resolveIntrinsic(codeURI, parameters)
		codeURI = ensureTrailingSlash(codeURI)

		handler := derefString(props.Handler, defaultHandler)
		runtime := derefString(props.Runtime, defaultRuntime)
		timeout := derefInt(props.Timeout, defaultTimeout)
		memory := derefInt(props.MemorySize, defaultMemory)

		envVars := map[string]string{}
		for key, value := range defaultEnv {
			envVars[key] = value
		}
		if env := asMap(props.Environment); env != nil {
			if vars := asMap(env["Variables"]); vars != nil {
				for key, raw := range vars {
					envVars[key] = resolveIntrinsic(asString(raw), parameters)
				}
			}
		}

		events := parseEvents(props.Events)

		scalingInput := map[string]any{}
		if props.ReservedConcurrentExecutions != nil {
			scalingInput["ReservedConcurrentExecutions"] = *props.ReservedConcurrentExecutions
		}
		if props.ProvisionedConcurrencyConfig != nil {
			pc := map[string]any{}
			if props.ProvisionedConcurrencyConfig.ProvisionedConcurrentExecutions != nil {
				pc["ProvisionedConcurrentExecutions"] = *props.ProvisionedConcurrencyConfig.ProvisionedConcurrentExecutions
			}
			scalingInput["ProvisionedConcurrencyConfig"] = pc
		}
		scaling := parseScaling(scalingInput)

		layerRefs := props.Layers
		if len(layerRefs) == 0 {
			layerRefs = defaultLayers
		}
		layers := collectLayers(layerRefs, layerMap)

		architectures := copyStringSlice(defaultArchitectures)
		if len(props.Architectures) > 0 {
			architectures = copyStringSlice(props.Architectures)
		}

		runtimeManagement := runtimeManagementFromProperties(props.RuntimeManagementConfig)
		if runtimeManagement.UpdateRuntimeOn == "" {
			runtimeManagement = defaultRuntimeManagement
		}

		functions = append(functions, FunctionSpec{
			LogicalID:               logicalID,
			Name:                    fnName,
			CodeURI:                 codeURI,
			Handler:                 handler,
			Runtime:                 runtime,
			Timeout:                 timeout,
			MemorySize:              memory,
			Environment:             envVars,
			Events:                  events,
			Scaling:                 scaling,
			Layers:                  layers,
			Architectures:           architectures,
			RuntimeManagementConfig: runtimeManagement,
		})
	}

	return ParseResult{Functions: functions, Resources: parsedResources}, nil
}

func parseEvents(events map[string]any) []EventSpec {
	if events == nil {
		return nil
	}
	result := []EventSpec{}
	for _, raw := range events {
		event := asMap(raw)
		if event == nil {
			continue
		}
		if asString(event["Type"]) != "Api" {
			continue
		}
		props := asMap(event["Properties"])
		if props == nil {
			continue
		}
		path := asString(props["Path"])
		method := asString(props["Method"])
		if path == "" || method == "" {
			continue
		}
		result = append(result, EventSpec{Path: path, Method: strings.ToLower(method)})
	}
	return result
}

func parseScaling(props map[string]any) ScalingSpec {
	var scaling ScalingSpec
	if value, ok := asIntPointer(props["ReservedConcurrentExecutions"]); ok {
		scaling.MaxCapacity = value
	}
	if provisioned := asMap(props["ProvisionedConcurrencyConfig"]); provisioned != nil {
		if value, ok := asIntPointer(provisioned["ProvisionedConcurrentExecutions"]); ok {
			scaling.MinCapacity = value
		}
	}
	return scaling
}

func collectLayers(raw any, layerMap map[string]LayerSpec) []LayerSpec {
	refs := extractLayerRefs(raw)
	if len(refs) == 0 {
		return nil
	}
	layers := make([]LayerSpec, 0, len(refs))
	for _, ref := range refs {
		if spec, ok := layerMap[ref]; ok {
			layers = append(layers, spec)
		}
	}
	return layers
}

func extractLayerRefs(raw any) []string {
	values := asSlice(raw)
	if values == nil {
		return nil
	}
	refs := make([]string, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case string:
			if typed != "" {
				refs = append(refs, typed)
			}
		case map[string]any:
			if ref := asString(typed["Ref"]); ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func runtimeManagementFromGlobals(config *schema.SamSchemaJsonGlobalsFunctionRuntimeManagementConfig) RuntimeManagementConfig {
	if config == nil || config.UpdateRuntimeOn == nil {
		return RuntimeManagementConfig{}
	}
	return RuntimeManagementConfig{UpdateRuntimeOn: *config.UpdateRuntimeOn}
}

func runtimeManagementFromProperties(config *schema.ResourcePropertiesRuntimeManagementConfig) RuntimeManagementConfig {
	if config == nil || config.UpdateRuntimeOn == nil {
		return RuntimeManagementConfig{}
	}
	return RuntimeManagementConfig{UpdateRuntimeOn: *config.UpdateRuntimeOn}
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func derefString(ptr *string, fallback string) string {
	if ptr != nil && *ptr != "" {
		return *ptr
	}
	return fallback
}

func derefInt(ptr *int, fallback int) int {
	if ptr != nil {
		return *ptr
	}
	return fallback
}
