// Where: cli/internal/infra/sam/template_defaults.go
// What: Defaults extraction for SAM function parsing.
// Why: Keep ParseSAMTemplate smaller and focused.
package sam

import (
	"strings"

	"github.com/poruru-code/esb/cli/internal/domain/value"
)

const (
	DefaultLambdaRuntime = "python3.12"
	DefaultLambdaHandler = "lambda_function.lambda_handler"
	DefaultLambdaTimeout = 30
	DefaultLambdaMemory  = 128
	DefaultCodeURI       = "./"
	DefaultBillingMode   = "PROVISIONED"
)

type functionDefaults struct {
	Runtime             string
	Handler             string
	Timeout             int
	Memory              int
	Layers              []any
	Architectures       []string
	RuntimeManagement   any
	EnvironmentDefaults map[string]string
}

func extractFunctionGlobals(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	globals := value.AsMap(data["Globals"])
	if globals == nil {
		return nil
	}
	return value.AsMap(globals["Function"])
}

func parseFunctionDefaults(functionGlobals map[string]any) functionDefaults {
	defaults := functionDefaults{
		Runtime:             DefaultLambdaRuntime,
		Handler:             DefaultLambdaHandler,
		Timeout:             DefaultLambdaTimeout,
		Memory:              DefaultLambdaMemory,
		EnvironmentDefaults: map[string]string{},
	}

	if functionGlobals == nil {
		return defaults
	}

	if val := functionGlobals["Runtime"]; val != nil {
		defaults.Runtime = value.AsString(val)
	}
	if val := functionGlobals["Handler"]; val != nil {
		defaults.Handler = value.AsString(val)
	}
	if val := functionGlobals["Timeout"]; val != nil {
		defaults.Timeout = value.AsInt(val)
	}
	if val := functionGlobals["MemorySize"]; val != nil {
		defaults.Memory = value.AsInt(val)
	}
	if layers := value.AsSlice(functionGlobals["Layers"]); layers != nil {
		defaults.Layers = layers
	}
	if archs := value.AsSlice(functionGlobals["Architectures"]); archs != nil {
		for _, a := range archs {
			defaults.Architectures = append(defaults.Architectures, value.AsString(a))
		}
	}
	defaults.RuntimeManagement = functionGlobals["RuntimeManagementConfig"]

	if env := value.AsMap(functionGlobals["Environment"]); env != nil {
		if vars := value.AsMap(env["Variables"]); vars != nil {
			for key, val := range vars {
				defaults.EnvironmentDefaults[key] = value.AsString(val)
			}
		}
	}

	return defaults
}

// Resolution helpers for standard AWS/SAM conventions

func ResolveTableName(props map[string]any, logicalID string) string {
	return value.AsStringDefault(props["TableName"], logicalID)
}

func ResolveS3BucketName(props map[string]any, logicalID string) string {
	// S3 bucket names are typically lowercase in SAM if not specified
	return value.AsStringDefault(props["BucketName"], strings.ToLower(logicalID))
}

func ResolveFunctionName(nameInProps any, logicalID string) string {
	return value.AsStringDefault(nameInProps, logicalID)
}

func ResolveCodeURI(uriInProps any) string {
	return value.AsStringDefault(uriInProps, DefaultCodeURI)
}

func ResolveBillingMode(props map[string]any) string {
	return value.AsStringDefault(props["BillingMode"], DefaultBillingMode)
}
