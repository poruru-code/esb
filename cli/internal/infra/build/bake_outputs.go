// Where: cli/internal/infra/build/bake_outputs.go
// What: Bake output/provenance resolution helpers.
// Why: Keep environment-driven output rules isolated from execution logic.
package build

import (
	"fmt"
	"os"
	"strings"
)

func bakeProvenanceArgs() []string {
	mode, ok := provenanceMode()
	if !ok {
		return nil
	}
	return []string{fmt.Sprintf("--provenance=%s", mode)}
}

func provenanceMode() (string, bool) {
	value := strings.TrimSpace(os.Getenv("PROVENANCE"))
	if value == "" {
		return "mode=max", true
	}
	switch strings.ToLower(value) {
	case "0", "false", "off", "no":
		return "", false
	case "1", "true", "on", "yes":
		return "mode=max", true
	default:
		return value, true
	}
}

func resolveBakeOutputs(registry string, pushToRegistry, includeDocker bool) []string {
	var outputs []string
	if includeDocker || !pushToRegistry {
		outputs = append(outputs, "type=docker")
	}
	if pushToRegistry && strings.TrimSpace(registry) != "" {
		output := "type=registry"
		if isInsecureRegistry(registry) {
			output += ",registry.insecure=true"
		}
		outputs = append(outputs, output)
	}
	return outputs
}

func isInsecureRegistry(registry string) bool {
	if strings.TrimSpace(registry) == "" {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY_INSECURE")))
	if value == "1" || value == "true" || value == "yes" {
		return true
	}
	host := registryHost(registry)
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "registry", "localhost", "127.0.0.1":
		return true
	default:
		return false
	}
}

func registryHost(registry string) string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	if slash := strings.Index(trimmed, "/"); slash != -1 {
		trimmed = trimmed[:slash]
	}
	host := trimmed
	if colon := strings.Index(host, ":"); colon != -1 {
		host = host[:colon]
	}
	return host
}
