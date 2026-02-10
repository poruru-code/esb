// Where: cli/internal/infra/sam/template_functions_serverless.go
// What: AWS::Serverless::Function parsing helpers.
// Why: Isolate serverless-function-specific decode and validation rules.
package sam

import (
	"fmt"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

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

	// Parse strict properties using parser.Decode.
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
		// Convert strictly typed ProvisionedConcurrencyConfig back to map.
		if pMap, ok := fnProps.ProvisionedConcurrencyConfig.(map[string]any); ok {
			scalingInput["ProvisionedConcurrencyConfig"] = pMap
		} else {
			// Try converting if it's map[interface{}]interface{} or other json types.
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
