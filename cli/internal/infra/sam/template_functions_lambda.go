// Where: cli/internal/infra/sam/template_functions_lambda.go
// What: AWS::Lambda::Function image-mode parsing helpers.
// Why: Keep lambda-specific image parsing and validation localized.
package sam

import (
	"fmt"
	"strings"

	"github.com/poruru-code/esb/cli/internal/domain/template"
	"github.com/poruru-code/esb/cli/internal/domain/value"
)

func parseLambdaFunction(
	logicalID string,
	resource map[string]any,
	defaults functionDefaults,
	warnf func(string, ...any),
) (template.FunctionSpec, bool, error) {
	props := value.AsMap(resource["Properties"])
	if props == nil {
		return template.FunctionSpec{}, false, nil
	}

	fnProps, err := DecodeLambdaFunctionProps(props)
	if err != nil {
		if warnf != nil {
			warnf("failed to map lambda properties for function %s: %v", logicalID, err)
		}
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
