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

func parseFunctionDefaults(functionGlobals map[string]any, ctx *ParserContext) functionDefaults {
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
		defaults.Runtime = ctx.asString(val)
	}
	if val := functionGlobals["Handler"]; val != nil {
		defaults.Handler = ctx.asString(val)
	}
	if val := functionGlobals["Timeout"]; val != nil {
		defaults.Timeout = ctx.asInt(val)
	}
	if val := functionGlobals["MemorySize"]; val != nil {
		defaults.Memory = ctx.asInt(val)
	}
	if layers := ctx.asSlice(functionGlobals["Layers"]); layers != nil {
		defaults.Layers = layers
	}
	if archs := ctx.asSlice(functionGlobals["Architectures"]); archs != nil {
		for _, a := range archs {
			defaults.Architectures = append(defaults.Architectures, ctx.asString(a))
		}
	}
	defaults.RuntimeManagement = functionGlobals["RuntimeManagementConfig"]

	if env := ctx.asMap(functionGlobals["Environment"]); env != nil {
		if vars := ctx.asMap(env["Variables"]); vars != nil {
			for key, value := range vars {
				defaults.EnvironmentDefaults[key] = ctx.asString(value)
			}
		}
	}

	return defaults
}
