package artifactcore

import (
	"fmt"
	"strings"
)

const RuntimeStackAPIVersion = "1.0"

type RuntimeObservation struct {
	Mode       string
	ESBVersion string
	Source     string
}

func validateRuntimeCompatibility(manifest ArtifactManifest, observation *RuntimeObservation) ([]string, error) {
	if !hasRuntimeStackRequirements(manifest.RuntimeStack) {
		return nil, nil
	}

	warnings := make([]string, 0)
	if err := validateAPIVersion(
		"runtime_stack.api_version",
		manifest.RuntimeStack.APIVersion,
		RuntimeStackAPIVersion,
		&warnings,
	); err != nil {
		return nil, err
	}

	obs := normalizeRuntimeObservation(observation)
	if obs == nil {
		warnings = append(warnings, "runtime stack observation is required when runtime_stack is set")
		return warnings, nil
	}

	requiredMode := strings.TrimSpace(manifest.RuntimeStack.Mode)
	if requiredMode != "" {
		if obs.Mode == "" {
			warnings = append(warnings, fmt.Sprintf("runtime_stack.mode expected %q but observed mode is empty", requiredMode))
		} else if !strings.EqualFold(requiredMode, obs.Mode) {
			return nil, fmt.Errorf("runtime_stack.mode mismatch: expected %q, observed %q", requiredMode, obs.Mode)
		}
	}

	return warnings, nil
}

func hasRuntimeStackRequirements(meta RuntimeStackMeta) bool {
	return strings.TrimSpace(meta.APIVersion) != "" || strings.TrimSpace(meta.Mode) != "" || strings.TrimSpace(meta.ESBVersion) != ""
}

func normalizeRuntimeObservation(observation *RuntimeObservation) *RuntimeObservation {
	if observation == nil {
		return nil
	}
	normalized := RuntimeObservation{
		Mode:       strings.TrimSpace(observation.Mode),
		ESBVersion: strings.TrimSpace(observation.ESBVersion),
		Source:     strings.TrimSpace(observation.Source),
	}
	if normalized.Mode == "" && normalized.ESBVersion == "" && normalized.Source == "" {
		return nil
	}
	return &normalized
}
