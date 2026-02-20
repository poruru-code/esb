package artifactcore

import (
	"strings"
)

type ApplyInput struct {
	ArtifactPath  string
	OutputDir     string
	SecretEnvPath string
	Runtime       *RuntimeObservation
}

type ApplyResult struct {
	Warnings []string
}

func ExecuteApply(input ApplyInput) (ApplyResult, error) {
	normalized, err := normalizeApplyInput(input)
	if err != nil {
		return ApplyResult{}, err
	}
	return executeApplyNormalized(normalized)
}

func normalizeApplyInput(input ApplyInput) (ApplyInput, error) {
	input.ArtifactPath = strings.TrimSpace(input.ArtifactPath)
	input.OutputDir = strings.TrimSpace(input.OutputDir)
	input.SecretEnvPath = strings.TrimSpace(input.SecretEnvPath)
	if input.ArtifactPath == "" {
		return ApplyInput{}, ErrArtifactPathRequired
	}
	if input.OutputDir == "" {
		return ApplyInput{}, ErrOutputDirRequired
	}
	return input, nil
}

func executeApplyNormalized(input ApplyInput) (ApplyResult, error) {
	warnings, err := applyWithWarnings(input)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Warnings: append([]string(nil), warnings...)}, nil
}
