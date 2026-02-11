// Where: cli/internal/infra/sam/template_functions_layers_runtime.go
// What: Shared layer/runtime/environment helpers for function parsing.
// Why: Keep reusable parsing primitives separate from resource-type handlers.
package sam

import (
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

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
