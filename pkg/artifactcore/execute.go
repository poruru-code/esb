package artifactcore

import (
	"strings"
)

type ApplyInput struct {
	ArtifactPath  string
	OutputDir     string
	SecretEnvPath string
	Strict        bool
}

type ApplyResult struct {
	Warnings []string
}

type DeployInput struct {
	Apply   ApplyInput
	NoCache bool
	Runner  CommandRunner
}

func ExecuteApply(input ApplyInput) (ApplyResult, error) {
	normalized, err := normalizeApplyInput(input)
	if err != nil {
		return ApplyResult{}, err
	}
	return executeApplyNormalized(normalized)
}

func ExecuteDeploy(input DeployInput) (ApplyResult, error) {
	normalized, err := normalizeApplyInput(input.Apply)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := prepareImages(prepareImagesInput{
		ArtifactPath: normalized.ArtifactPath,
		NoCache:      input.NoCache,
		Runner:       input.Runner,
	}); err != nil {
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
