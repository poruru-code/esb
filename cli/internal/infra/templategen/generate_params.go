// Where: cli/internal/infra/templategen/generate_params.go
// What: Parameter and ordering helpers for generation flow.
// Why: Isolate pure helper logic from GenerateFiles orchestration.
package templategen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/poruru-code/esb/cli/internal/domain/template"
)

// resolveTag picks the Docker image tag from opts first, then generator config, then "latest".
func resolveTag(tag, fallback string) string {
	if strings.TrimSpace(tag) != "" {
		return tag
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "latest"
}

func sortFunctionsByName(functions []template.FunctionSpec) {
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Name < functions[j].Name
	})
}

// mergeParameters merges generator config parameters with runtime overrides.
func mergeParameters(cfgParams map[string]any, overrides map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range cfgParams {
		if value == nil {
			continue
		}
		out[key] = fmt.Sprint(value)
	}
	for key, value := range overrides {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	return out
}
