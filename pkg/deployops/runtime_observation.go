package deployops

import "strings"

type RuntimeObservation struct {
	Mode       string
	ESBVersion string
	Source     string
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
