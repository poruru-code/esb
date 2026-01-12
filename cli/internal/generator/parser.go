// Where: cli/internal/generator/parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package generator

import (
	"encoding/json"
	"strings"

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

	resources := asMap(data["Resources"])
	if resources == nil {
		return ParseResult{}, nil
	}

	// Parse Globals using raw map access for reliability
	var functionGlobals map[string]any
	if data["Globals"] != nil {
		globals := asMap(data["Globals"])
		if globals != nil {
			functionGlobals = asMap(globals["Function"])
		}
	}

	defaultRuntime := "python3.12"
	defaultHandler := "lambda_function.lambda_handler"
	defaultTimeout := 30
	defaultMemory := 128
	var defaultLayers []any
	var defaultArchitectures []string
	var defaultRuntimeManagement any

	if functionGlobals != nil {
		if val := functionGlobals["Runtime"]; val != nil {
			defaultRuntime = asString(val)
		}
		if val := functionGlobals["Handler"]; val != nil {
			defaultHandler = asString(val)
		}
		if val := functionGlobals["Timeout"]; val != nil {
			defaultTimeout = asInt(val)
		}
		if val := functionGlobals["MemorySize"]; val != nil {
			defaultMemory = asInt(val)
		}
		if layers := asSlice(functionGlobals["Layers"]); layers != nil {
			defaultLayers = layers
		}
		if archs := asSlice(functionGlobals["Architectures"]); archs != nil {
			for _, a := range archs {
				defaultArchitectures = append(defaultArchitectures, asString(a))
			}
		}
		defaultRuntimeManagement = functionGlobals["RuntimeManagementConfig"]
	}

	defaultEnv := map[string]string{}
	if functionGlobals != nil {
		if env := asMap(functionGlobals["Environment"]); env != nil {
			if vars := asMap(env["Variables"]); vars != nil {
				for key, value := range vars {
					defaultEnv[key] = resolveIntrinsic(asString(value), parameters)
				}
			}
		}
	}

	parsedResources := ResourcesSpec{}
	layerMap := map[string]LayerSpec{}

	// First pass: collect Layers
	for logicalID, resource := range template.Resources {
		m := asMap(resource)
		if m == nil || asString(m["Type"]) != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := asMap(m["Properties"])
		if props == nil {
			continue
		}
		layerName := asStringDefault(props["LayerName"], logicalID)
		layerName = resolveIntrinsic(layerName, parameters)
		contentURI := asStringDefault(props["ContentUri"], "./")
		contentURI = resolveIntrinsic(contentURI, parameters)
		contentURI = ensureTrailingSlash(contentURI)

		var compatibleArchs []string
		if archs := asSlice(props["CompatibleArchitectures"]); archs != nil {
			for _, a := range archs {
				compatibleArchs = append(compatibleArchs, asString(a))
			}
		}

		spec := LayerSpec{
			Name:                    layerName,
			ContentURI:              contentURI,
			CompatibleArchitectures: compatibleArchs,
		}
		layerMap[logicalID] = spec
		parsedResources.Layers = append(parsedResources.Layers, spec)
	}

	// Second pass: collect other resources
	for logicalID, value := range template.Resources {
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
			tableName := asStringDefault(props["TableName"], logicalID)
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
			bucketName := asStringDefault(props["BucketName"], strings.ToLower(logicalID))
			bucketName = resolveIntrinsic(bucketName, parameters)
			parsedResources.S3 = append(parsedResources.S3, S3Spec{BucketName: bucketName})
		}
	}

	// Third pass: collect Functions
	functions := make([]FunctionSpec, 0)
	for logicalID, value := range template.Resources {
		m := asMap(value)
		if m == nil || asString(m["Type"]) != "AWS::Serverless::Function" {
			continue
		}

		props := asMap(m["Properties"])
		if props == nil {
			continue
		}

		fnName := asStringDefault(props["FunctionName"], logicalID)
		fnName = resolveIntrinsic(fnName, parameters)
		codeURI := asStringDefault(props["CodeUri"], "./")
		codeURI = resolveIntrinsic(codeURI, parameters)
		codeURI = ensureTrailingSlash(codeURI)

		handler := asStringDefault(props["Handler"], defaultHandler)
		runtime := asStringDefault(props["Runtime"], defaultRuntime)
		timeout := asIntDefault(props["Timeout"], defaultTimeout)
		memory := asIntDefault(props["MemorySize"], defaultMemory)

		envVars := map[string]string{}
		for key, value := range defaultEnv {
			envVars[key] = value
		}
		if env := asMap(props["Environment"]); env != nil {
			if vars := asMap(env["Variables"]); vars != nil {
				for key, val := range vars {
					envVars[key] = resolveIntrinsic(asString(val), parameters)
				}
			}
		}

		events := parseEvents(asMap(props["Events"]))

		scalingInput := map[string]any{}
		if val := props["ReservedConcurrentExecutions"]; val != nil {
			scalingInput["ReservedConcurrentExecutions"] = val
		}
		if provisioned := asMap(props["ProvisionedConcurrencyConfig"]); provisioned != nil {
			scalingInput["ProvisionedConcurrencyConfig"] = provisioned
		}
		scaling := parseScaling(scalingInput)

		layerRefs := props["Layers"]
		if layerRefs == nil {
			layerRefs = defaultLayers
		}
		layers := collectLayers(layerRefs, layerMap)

		var architectures []string
		if archs := asSlice(props["Architectures"]); archs != nil {
			for _, a := range archs {
				architectures = append(architectures, asString(a))
			}
		} else {
			architectures = copyStringSlice(defaultArchitectures)
		}

		runtimeManagement := runtimeManagementFromProperties(props["RuntimeManagementConfig"])
		if runtimeManagement.UpdateRuntimeOn == "" && defaultRuntimeManagement != nil {
			runtimeManagement = runtimeManagementFromGlobals(defaultRuntimeManagement)
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
		eventType := asString(event["Type"])
		props := asMap(event["Properties"])
		if props == nil {
			continue
		}

		if eventType == "Api" {
			path := asString(props["Path"])
			method := asString(props["Method"])
			if path == "" || method == "" {
				continue
			}
			result = append(result, EventSpec{
				Type:   "Api",
				Path:   path,
				Method: strings.ToLower(method),
			})
		} else if eventType == "Schedule" {
			schedule := asString(props["Schedule"])
			if schedule == "" {
				continue
			}
			input := asString(props["Input"])
			result = append(result, EventSpec{
				Type:               "Schedule",
				ScheduleExpression: schedule,
				Input:              input,
			})
		}
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

func runtimeManagementFromGlobals(config any) RuntimeManagementConfig {
	m := asMap(config)
	if m == nil || m["UpdateRuntimeOn"] == nil {
		return RuntimeManagementConfig{}
	}
	return RuntimeManagementConfig{UpdateRuntimeOn: asString(m["UpdateRuntimeOn"])}
}

func runtimeManagementFromProperties(config any) RuntimeManagementConfig {
	m := asMap(config)
	if m == nil || m["UpdateRuntimeOn"] == nil {
		return RuntimeManagementConfig{}
	}
	return RuntimeManagementConfig{UpdateRuntimeOn: asString(m["UpdateRuntimeOn"])}
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}
