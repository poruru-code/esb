// Where: cli/internal/generator/parser_defaults.go
// What: Defaults extraction for SAM function parsing.
// Why: Keep ParseSAMTemplate smaller and focused.
package generator

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
	globals := asMap(data["Globals"])
	if globals == nil {
		return nil
	}
	return asMap(globals["Function"])
}

func parseFunctionDefaults(functionGlobals map[string]any, parameters map[string]string) functionDefaults {
	defaults := functionDefaults{
		Runtime:             "python3.12",
		Handler:             "lambda_function.lambda_handler",
		Timeout:             30,
		Memory:              128,
		EnvironmentDefaults: map[string]string{},
	}

	if functionGlobals == nil {
		return defaults
	}

	if val := functionGlobals["Runtime"]; val != nil {
		defaults.Runtime = asString(val)
	}
	if val := functionGlobals["Handler"]; val != nil {
		defaults.Handler = asString(val)
	}
	if val := functionGlobals["Timeout"]; val != nil {
		defaults.Timeout = asInt(val)
	}
	if val := functionGlobals["MemorySize"]; val != nil {
		defaults.Memory = asInt(val)
	}
	if layers := asSlice(functionGlobals["Layers"]); layers != nil {
		defaults.Layers = layers
	}
	if archs := asSlice(functionGlobals["Architectures"]); archs != nil {
		for _, a := range archs {
			defaults.Architectures = append(defaults.Architectures, asString(a))
		}
	}
	defaults.RuntimeManagement = functionGlobals["RuntimeManagementConfig"]

	if env := asMap(functionGlobals["Environment"]); env != nil {
		if vars := asMap(env["Variables"]); vars != nil {
			for key, value := range vars {
				defaults.EnvironmentDefaults[key] = resolveIntrinsic(asString(value), parameters)
			}
		}
	}

	return defaults
}
