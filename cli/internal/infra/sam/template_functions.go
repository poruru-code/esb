// Where: cli/internal/infra/sam/template_functions.go
// What: Function parsing helpers for SAM templates.
// Why: Keep function extraction logic isolated and testable.
package sam

import (
	"fmt"
	"strings"

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

func parseServerlessFunction(
	logicalID string,
	resource map[string]any,
	defaults functionDefaults,
	layerMap map[string]manifest.LayerSpec,
) (template.FunctionSpec, bool, error) {
	props := value.AsMap(resource["Properties"])
	if props == nil {
		return template.FunctionSpec{}, false, nil
	}

	// Parse strict properties using parser.Decode
	fnProps, err := DecodeFunctionProps(props)
	if err != nil {
		fmt.Printf("Warning: failed to map properties for function %s: %v\n", logicalID, err)
	}

	fnName := ResolveFunctionName(fnProps.FunctionName, logicalID)
	timeout := value.AsIntDefault(fnProps.Timeout, defaults.Timeout)
	memory := value.AsIntDefault(fnProps.MemorySize, defaults.Memory)
	envVars := mergeEnv(defaults.EnvironmentDefaults, props)
	architectures := resolveArchitectures(props, defaults.Architectures)

	isImageFunction := strings.EqualFold(value.AsString(fnProps.PackageType), "Image") ||
		strings.TrimSpace(value.AsString(fnProps.ImageURI)) != ""
	if isImageFunction {
		imageURI := strings.TrimSpace(value.AsString(fnProps.ImageURI))
		if imageURI == "" {
			return template.FunctionSpec{}, false, fmt.Errorf(
				"image function %s (%s) requires ImageUri",
				fnName,
				logicalID,
			)
		}
		if hasUnresolvedImageURI(imageURI) {
			return template.FunctionSpec{}, false, fmt.Errorf(
				"image function %s (%s) has unresolved ImageUri: %s",
				fnName,
				logicalID,
				imageURI,
			)
		}
		return template.FunctionSpec{
			LogicalID:       logicalID,
			Name:            fnName,
			ImageSource:     imageURI,
			Timeout:         timeout,
			MemorySize:      memory,
			Environment:     envVars,
			Architectures:   architectures,
			Scaling:         parseScaling(props),
			Events:          parseEvents(value.AsMap(fnProps.Events)),
			Layers:          nil,
			Runtime:         "",
			Handler:         "",
			CodeURI:         "",
			HasRequirements: false,
		}, true, nil
	}

	codeURI := ResolveCodeURI(fnProps.CodeURI)
	codeURI = value.EnsureTrailingSlash(codeURI)
	handler := value.AsStringDefault(fnProps.Handler, defaults.Handler)
	runtime := value.AsStringDefault(fnProps.Runtime, defaults.Runtime)

	if fnProps.Events != nil {
		eventsRaw, err := DecodeMap(fnProps.Events)
		if err == nil {
			fnProps.Events = eventsRaw
		}
	}
	events := parseEvents(value.AsMap(fnProps.Events))

	scalingInput := map[string]any{}
	if val := fnProps.ReservedConcurrentExecutions; val != nil {
		scalingInput["ReservedConcurrentExecutions"] = val
	}
	if fnProps.ProvisionedConcurrencyConfig != nil {
		// Convert strictly typed ProvisionedConcurrencyConfig back to map?
		// Actually, schema definition says ProvisionedConcurrencyConfig interface{}
		if pMap, ok := fnProps.ProvisionedConcurrencyConfig.(map[string]any); ok {
			scalingInput["ProvisionedConcurrencyConfig"] = pMap
		} else {
			// Try converting if it's map[interface{}]interface{} or other json types
			if converted, err := DecodeMap(fnProps.ProvisionedConcurrencyConfig); err == nil {
				scalingInput["ProvisionedConcurrencyConfig"] = converted
			}
		}
	}
	scaling := parseScaling(scalingInput)

	layerRefs := fnProps.Layers
	if layerRefs == nil {
		layerRefs = defaults.Layers
	}
	layers := collectLayers(layerRefs, layerMap)

	runtimeManagement := runtimeManagementFromConfig(fnProps.RuntimeManagementConfig)
	if runtimeManagement.UpdateRuntimeOn == "" && defaults.RuntimeManagement != nil {
		runtimeManagement = runtimeManagementFromConfig(defaults.RuntimeManagement)
	}

	return template.FunctionSpec{
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
	}, true, nil
}

func parseLambdaFunction(
	logicalID string,
	resource map[string]any,
	defaults functionDefaults,
) (template.FunctionSpec, bool, error) {
	props := value.AsMap(resource["Properties"])
	if props == nil {
		return template.FunctionSpec{}, false, nil
	}

	fnProps, err := DecodeLambdaFunctionProps(props)
	if err != nil {
		fmt.Printf("Warning: failed to map lambda properties for function %s: %v\n", logicalID, err)
	}

	packageType := strings.TrimSpace(value.AsString(fnProps.PackageType))
	imageURI := strings.TrimSpace(value.AsString(fnProps.Code.ImageURI))
	if !strings.EqualFold(packageType, "Image") && imageURI == "" {
		// AWS::Lambda::Function Zip package is out of scope.
		return template.FunctionSpec{}, false, nil
	}
	if imageURI == "" {
		return template.FunctionSpec{}, false, fmt.Errorf(
			"image lambda function %s (%s) requires Code.ImageUri",
			ResolveFunctionName(fnProps.FunctionName, logicalID),
			logicalID,
		)
	}
	if hasUnresolvedImageURI(imageURI) {
		return template.FunctionSpec{}, false, fmt.Errorf(
			"image lambda function %s (%s) has unresolved Code.ImageUri: %s",
			ResolveFunctionName(fnProps.FunctionName, logicalID),
			logicalID,
			imageURI,
		)
	}

	fnName := ResolveFunctionName(fnProps.FunctionName, logicalID)
	timeout := value.AsIntDefault(fnProps.Timeout, defaults.Timeout)
	memory := value.AsIntDefault(fnProps.MemorySize, defaults.Memory)
	envVars := mergeEnv(defaults.EnvironmentDefaults, props)
	architectures := resolveArchitectures(props, defaults.Architectures)

	return template.FunctionSpec{
		LogicalID:       logicalID,
		Name:            fnName,
		ImageSource:     imageURI,
		Timeout:         timeout,
		MemorySize:      memory,
		Environment:     envVars,
		Scaling:         parseScaling(props),
		Architectures:   architectures,
		Layers:          nil,
		Events:          nil,
		Runtime:         "",
		Handler:         "",
		CodeURI:         "",
		HasRequirements: false,
	}, true, nil
}

func mergeEnv(defaultEnv map[string]string, props map[string]any) map[string]string {
	envVars := map[string]string{}
	for key, val := range defaultEnv {
		envVars[key] = val
	}
	if env := value.AsMap(props["Environment"]); env != nil {
		if vars := value.AsMap(env["Variables"]); vars != nil {
			for key, val := range vars {
				envVars[key] = value.AsString(val)
			}
		}
	}
	return envVars
}

func resolveArchitectures(props map[string]any, defaults []string) []string {
	if archs := value.AsSlice(props["Architectures"]); archs != nil {
		var architectures []string
		for _, a := range archs {
			architectures = append(architectures, value.AsString(a))
		}
		return architectures
	}
	return copyStringSlice(defaults)
}

func parseEvents(events map[string]any) []template.EventSpec {
	if events == nil {
		return nil
	}
	result := []template.EventSpec{}
	for _, raw := range events {
		event := value.AsMap(raw)
		if event == nil {
			continue
		}
		eventType := value.AsString(event["Type"])
		props := value.AsMap(event["Properties"])
		if props == nil {
			continue
		}

		if eventType == "Api" {
			path := value.AsString(props["Path"])
			method := value.AsString(props["Method"])
			if path == "" || method == "" {
				continue
			}
			result = append(result, template.EventSpec{
				Type:   "Api",
				Path:   path,
				Method: strings.ToLower(method),
			})
		} else if eventType == "Schedule" {
			schedule := value.AsString(props["Schedule"])
			if schedule == "" {
				continue
			}
			input := value.AsString(props["Input"])
			result = append(result, template.EventSpec{
				Type:               "Schedule",
				ScheduleExpression: schedule,
				Input:              input,
			})
		}
	}
	return result
}

func parseScaling(props map[string]any) template.ScalingSpec {
	var scaling template.ScalingSpec
	if value, ok := value.AsIntPointer(props["ReservedConcurrentExecutions"]); ok {
		scaling.MaxCapacity = value
	}
	if provisioned := value.AsMap(props["ProvisionedConcurrencyConfig"]); provisioned != nil {
		if value, ok := value.AsIntPointer(provisioned["ProvisionedConcurrentExecutions"]); ok {
			scaling.MinCapacity = value
		}
	}
	return scaling
}

func collectLayers(raw any, layerMap map[string]manifest.LayerSpec) []manifest.LayerSpec {
	refs := extractLayerRefs(raw)
	if len(refs) == 0 {
		return nil
	}
	layers := make([]manifest.LayerSpec, 0, len(refs))
	for _, ref := range refs {
		if spec, ok := layerMap[ref]; ok {
			layers = append(layers, spec)
		}
	}
	return layers
}

func extractLayerRefs(raw any) []string {
	values := value.AsSlice(raw)
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
			if ref := value.AsString(typed["Ref"]); ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func runtimeManagementFromConfig(config any) template.RuntimeManagementConfig {
	m := value.AsMap(config)
	if m == nil || m["UpdateRuntimeOn"] == nil {
		return template.RuntimeManagementConfig{}
	}
	return template.RuntimeManagementConfig{UpdateRuntimeOn: value.AsString(m["UpdateRuntimeOn"])}
}

func hasUnresolvedImageURI(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.Contains(trimmed, "${")
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}
