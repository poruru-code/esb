// Where: cli/internal/generator/parser_functions.go
// What: Function parsing helpers for SAM templates.
// Why: Keep function extraction logic isolated and testable.
package generator

import (
	"fmt"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
)

func parseFunctions(
	resources map[string]any,
	defaults functionDefaults,
	layerMap map[string]LayerSpec,
	ctx *ParserContext,
) []FunctionSpec {
	functions := make([]FunctionSpec, 0)
	for logicalID, value := range resources {
		m := ctx.asMap(value)
		if m == nil || ctx.asString(m["Type"]) != "AWS::Serverless::Function" {
			continue
		}

		props := ctx.asMap(m["Properties"])
		if props == nil {
			continue
		}

		// Parse strict properties using mapToStruct
		var fnProps schema.SamtranslatorInternalSchemaSourceAwsServerlessFunctionProperties
		if err := ctx.mapToStruct(props, &fnProps); err != nil {
			// Report error but continue with what we have
			fmt.Printf("Warning: failed to map properties for function %s: %v\n", logicalID, err)
		}

		fnName := ResolveFunctionName(fnProps.FunctionName, logicalID, ctx)
		codeURI := ResolveCodeURI(fnProps.CodeUri, ctx)
		codeURI = ensureTrailingSlash(codeURI)

		handler := ctx.asStringDefault(fnProps.Handler, defaults.Handler)
		runtime := ctx.asStringDefault(fnProps.Runtime, defaults.Runtime)
		timeout := ctx.asIntDefault(fnProps.Timeout, defaults.Timeout)
		memory := ctx.asIntDefault(fnProps.MemorySize, defaults.Memory)

		envVars := mergeEnv(defaults.EnvironmentDefaults, props, ctx)

		if fnProps.Events != nil {
			eventsRaw := make(map[string]any)
			// Map to direct map to support parseEvents
			if err := ctx.mapToStruct(fnProps.Events, &eventsRaw); err == nil {
				fnProps.Events = eventsRaw
			}
		}
		events := parseEvents(asMap(fnProps.Events), ctx)

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
				if err := ctx.mapToStruct(fnProps.ProvisionedConcurrencyConfig, &converted); err == nil {
					scalingInput["ProvisionedConcurrencyConfig"] = converted
				}
			}
		}
		scaling := parseScaling(scalingInput, ctx)

		layerRefs := fnProps.Layers
		if layerRefs == nil {
			layerRefs = defaults.Layers
		}
		layers := collectLayers(layerRefs, layerMap, ctx)

		architectures := resolveArchitectures(props, defaults.Architectures, ctx)

		runtimeManagement := runtimeManagementFromConfig(fnProps.RuntimeManagementConfig, ctx)
		if runtimeManagement.UpdateRuntimeOn == "" && defaults.RuntimeManagement != nil {
			runtimeManagement = runtimeManagementFromConfig(defaults.RuntimeManagement, ctx)
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

func mergeEnv(defaultEnv map[string]string, props map[string]any, ctx *ParserContext) map[string]string {
	envVars := map[string]string{}
	for key, value := range defaultEnv {
		envVars[key] = value
	}
	if env := ctx.asMap(props["Environment"]); env != nil {
		if vars := ctx.asMap(env["Variables"]); vars != nil {
			for key, val := range vars {
				envVars[key] = ctx.asString(val)
			}
		}
	}
	return envVars
}

func resolveArchitectures(props map[string]any, defaults []string, ctx *ParserContext) []string {
	if archs := ctx.asSlice(props["Architectures"]); archs != nil {
		var architectures []string
		for _, a := range archs {
			architectures = append(architectures, ctx.asString(a))
		}
		return architectures
	}
	return copyStringSlice(defaults)
}

func parseEvents(events map[string]any, ctx *ParserContext) []EventSpec {
	if events == nil {
		return nil
	}
	result := []EventSpec{}
	for _, raw := range events {
		event := ctx.asMap(raw)
		if event == nil {
			continue
		}
		eventType := ctx.asString(event["Type"])
		props := ctx.asMap(event["Properties"])
		if props == nil {
			continue
		}

		if eventType == "Api" {
			path := ctx.asString(props["Path"])
			method := ctx.asString(props["Method"])
			if path == "" || method == "" {
				continue
			}
			result = append(result, EventSpec{
				Type:   "Api",
				Path:   path,
				Method: strings.ToLower(method),
			})
		} else if eventType == "Schedule" {
			schedule := ctx.asString(props["Schedule"])
			if schedule == "" {
				continue
			}
			input := ctx.asString(props["Input"])
			result = append(result, EventSpec{
				Type:               "Schedule",
				ScheduleExpression: schedule,
				Input:              input,
			})
		}
	}
	return result
}

func parseScaling(props map[string]any, ctx *ParserContext) ScalingSpec {
	var scaling ScalingSpec
	if value, ok := ctx.asIntPointer(props["ReservedConcurrentExecutions"]); ok {
		scaling.MaxCapacity = value
	}
	if provisioned := ctx.asMap(props["ProvisionedConcurrencyConfig"]); provisioned != nil {
		if value, ok := ctx.asIntPointer(provisioned["ProvisionedConcurrentExecutions"]); ok {
			scaling.MinCapacity = value
		}
	}
	return scaling
}

func collectLayers(raw any, layerMap map[string]LayerSpec, ctx *ParserContext) []LayerSpec {
	refs := extractLayerRefs(raw, ctx)
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

func extractLayerRefs(raw any, ctx *ParserContext) []string {
	values := ctx.asSlice(raw)
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
			if ref := ctx.asString(typed["Ref"]); ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func runtimeManagementFromConfig(config any, ctx *ParserContext) RuntimeManagementConfig {
	m := ctx.asMap(config)
	if m == nil || m["UpdateRuntimeOn"] == nil {
		return RuntimeManagementConfig{}
	}
	return RuntimeManagementConfig{UpdateRuntimeOn: ctx.asString(m["UpdateRuntimeOn"])}
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}
