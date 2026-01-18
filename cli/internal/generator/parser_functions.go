// Where: cli/internal/generator/parser_functions.go
// What: Function parsing helpers for SAM templates.
// Why: Keep function extraction logic isolated and testable.
package generator

import (
	"fmt"
	"strings"

	samparser "github.com/poruru-code/aws-sam-parser-go/parser"
	"github.com/poruru-code/aws-sam-parser-go/schema"
)

func parseFunctions(
	resources map[string]any,
	defaults functionDefaults,
	layerMap map[string]LayerSpec,
) []FunctionSpec {
	functions := make([]FunctionSpec, 0)
	for logicalID, value := range resources {
		m := asMap(value)
		if m == nil || asString(m["Type"]) != "AWS::Serverless::Function" {
			continue
		}

		props := asMap(m["Properties"])
		if props == nil {
			continue
		}

		// Parse strict properties using parser.Decode
		var fnProps schema.SamtranslatorInternalSchemaSourceAwsServerlessFunctionProperties
		if err := samparser.Decode(props, &fnProps, nil); err != nil {
			// Report error but continue with what we have
			fmt.Printf("Warning: failed to map properties for function %s: %v\n", logicalID, err)
		}

		fnName := ResolveFunctionName(fnProps.FunctionName, logicalID)
		codeURI := ResolveCodeURI(fnProps.CodeUri)
		codeURI = ensureTrailingSlash(codeURI)

		handler := asStringDefault(fnProps.Handler, defaults.Handler)
		runtime := asStringDefault(fnProps.Runtime, defaults.Runtime)
		timeout := asIntDefault(fnProps.Timeout, defaults.Timeout)
		memory := asIntDefault(fnProps.MemorySize, defaults.Memory)

		envVars := mergeEnv(defaults.EnvironmentDefaults, props)

		if fnProps.Events != nil {
			eventsRaw := make(map[string]any)
			// Map to direct map to support parseEvents
			if err := samparser.Decode(fnProps.Events, &eventsRaw, nil); err == nil {
				fnProps.Events = eventsRaw
			}
		}
		events := parseEvents(asMap(fnProps.Events))

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
				var converted map[string]any
				if err := samparser.Decode(fnProps.ProvisionedConcurrencyConfig, &converted, nil); err == nil {
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

		architectures := resolveArchitectures(props, defaults.Architectures)

		runtimeManagement := runtimeManagementFromConfig(fnProps.RuntimeManagementConfig)
		if runtimeManagement.UpdateRuntimeOn == "" && defaults.RuntimeManagement != nil {
			runtimeManagement = runtimeManagementFromConfig(defaults.RuntimeManagement)
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

	return functions
}

func mergeEnv(defaultEnv map[string]string, props map[string]any) map[string]string {
	envVars := map[string]string{}
	for key, value := range defaultEnv {
		envVars[key] = value
	}
	if env := asMap(props["Environment"]); env != nil {
		if vars := asMap(env["Variables"]); vars != nil {
			for key, val := range vars {
				envVars[key] = asString(val)
			}
		}
	}
	return envVars
}

func resolveArchitectures(props map[string]any, defaults []string) []string {
	if archs := asSlice(props["Architectures"]); archs != nil {
		var architectures []string
		for _, a := range archs {
			architectures = append(architectures, asString(a))
		}
		return architectures
	}
	return copyStringSlice(defaults)
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

func runtimeManagementFromConfig(config any) RuntimeManagementConfig {
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
