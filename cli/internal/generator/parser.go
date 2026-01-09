// Where: cli/internal/generator/parser.go
// What: SAM template parser for Go generator.
// Why: Replace Python parser with a typed, testable implementation.
package generator

import "strings"

type ParseResult struct {
	Functions []FunctionSpec
	Resources ResourcesSpec
}

type FunctionSpec struct {
	LogicalID       string
	Name            string
	CodeURI         string
	Handler         string
	Runtime         string
	Timeout         int
	MemorySize      int
	HasRequirements bool
	Environment     map[string]string
	Events          []EventSpec
	Scaling         ScalingSpec
	Layers          []LayerSpec
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
	Name       string
	ContentURI string
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

	data, err := decodeYAML(content)
	if err != nil {
		return ParseResult{}, err
	}

	globals := asMap(data["Globals"])
	functionGlobals := asMap(globals["Function"])

	defaultRuntime := asString(functionGlobals["Runtime"])
	if defaultRuntime == "" {
		defaultRuntime = "python3.12"
	}
	defaultHandler := asString(functionGlobals["Handler"])
	if defaultHandler == "" {
		defaultHandler = "lambda_function.lambda_handler"
	}
	defaultTimeout := asIntDefault(functionGlobals["Timeout"], 30)
	defaultMemory := asIntDefault(functionGlobals["MemorySize"], 128)
	defaultLayers := asSlice(functionGlobals["Layers"])
	defaultEnv := map[string]string{}
	if env := asMap(functionGlobals["Environment"]); env != nil {
		if vars := asMap(env["Variables"]); vars != nil {
			for key, raw := range vars {
				defaultEnv[key] = resolveIntrinsic(asString(raw), parameters)
			}
		}
	}

	resources := asMap(data["Resources"])
	if resources == nil {
		return ParseResult{}, nil
	}

	layerMap := map[string]LayerSpec{}
	parsedResources := ResourcesSpec{}
	functions := make([]FunctionSpec, 0)

	for logicalID, value := range resources {
		resource := asMap(value)
		if resource == nil {
			continue
		}
		resourceType := asString(resource["Type"])
		props := asMap(resource["Properties"])

		switch resourceType {
		case "AWS::Serverless::LayerVersion":
			layerName := asString(props["LayerName"])
			if layerName == "" {
				layerName = logicalID
			}
			layerName = resolveIntrinsic(layerName, parameters)
			contentURI := asString(props["ContentUri"])
			if contentURI == "" {
				contentURI = "./"
			}
			contentURI = resolveIntrinsic(contentURI, parameters)
			contentURI = ensureTrailingSlash(contentURI)
			spec := LayerSpec{Name: layerName, ContentURI: contentURI}
			layerMap[logicalID] = spec
			parsedResources.Layers = append(parsedResources.Layers, spec)
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

	for logicalID, value := range resources {
		resource := asMap(value)
		if resource == nil {
			continue
		}
		if asString(resource["Type"]) != "AWS::Serverless::Function" {
			continue
		}
		props := asMap(resource["Properties"])

		fnName := asString(props["FunctionName"])
		if fnName == "" {
			fnName = logicalID
		}
		fnName = resolveIntrinsic(fnName, parameters)
		codeURI := asString(props["CodeUri"])
		if codeURI == "" {
			codeURI = "./"
		}
		codeURI = resolveIntrinsic(codeURI, parameters)
		codeURI = ensureTrailingSlash(codeURI)

		handler := asString(props["Handler"])
		if handler == "" {
			handler = defaultHandler
		}
		runtime := asString(props["Runtime"])
		if runtime == "" {
			runtime = defaultRuntime
		}

		envVars := map[string]string{}
		for key, value := range defaultEnv {
			envVars[key] = value
		}
		if env := asMap(props["Environment"]); env != nil {
			if vars := asMap(env["Variables"]); vars != nil {
				for key, raw := range vars {
					envVars[key] = resolveIntrinsic(asString(raw), parameters)
				}
			}
		}

		events := parseEvents(asMap(props["Events"]))
		scaling := parseScaling(props)

		layerRefs := props["Layers"]
		if layerRefs == nil {
			layerRefs = defaultLayers
		}
		layers := collectLayers(layerRefs, layerMap)

		functions = append(functions, FunctionSpec{
			LogicalID:   logicalID,
			Name:        fnName,
			CodeURI:     codeURI,
			Handler:     handler,
			Runtime:     runtime,
			Timeout:     asIntDefault(props["Timeout"], defaultTimeout),
			MemorySize:  asIntDefault(props["MemorySize"], defaultMemory),
			Environment: envVars,
			Events:      events,
			Scaling:     scaling,
			Layers:      layers,
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
