package deployops

import (
	"fmt"
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type Input struct {
	ArtifactPath  string
	OutputDir     string
	SecretEnvPath string
	NoCache       bool
	Runner        CommandRunner
	RuntimeProbe  RuntimeProbe
}

type RuntimeProbe func(manifest artifactcore.ArtifactManifest) (*artifactcore.RuntimeObservation, []string, error)

func Execute(input Input) (artifactcore.ApplyResult, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	manifest, err := artifactcore.ReadArtifactManifest(normalized.ArtifactPath)
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	var (
		observation *artifactcore.RuntimeObservation
		warnings    []string
	)
	if hasRuntimeStackRequirements(manifest.RuntimeStack) {
		probe := normalized.RuntimeProbe
		if probe == nil {
			probe = probeRuntimeObservation
		}
		observed, probeWarnings, probeErr := probe(manifest)
		if probeErr != nil {
			probeWarnings = append(
				probeWarnings,
				fmt.Sprintf("runtime compatibility probe failed: %v", probeErr),
			)
		}
		observation = observed
		warnings = append(warnings, probeWarnings...)
	}
	if err := prepareImages(prepareImagesInput{
		ArtifactPath: normalized.ArtifactPath,
		NoCache:      normalized.NoCache,
		Runner:       normalized.Runner,
		Runtime:      observation,
		EnsureBase:   true,
	}); err != nil {
		return artifactcore.ApplyResult{}, err
	}

	result, err := artifactcore.ExecuteApply(artifactcore.ApplyInput{
		ArtifactPath:  normalized.ArtifactPath,
		OutputDir:     normalized.OutputDir,
		SecretEnvPath: normalized.SecretEnvPath,
		Runtime:       observation,
	})
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	result.Warnings = append(warnings, result.Warnings...)
	return result, nil
}

func hasRuntimeStackRequirements(meta artifactcore.RuntimeStackMeta) bool {
	return strings.TrimSpace(meta.APIVersion) != "" ||
		strings.TrimSpace(meta.Mode) != "" ||
		strings.TrimSpace(meta.ESBVersion) != ""
}

func normalizeInput(input Input) (Input, error) {
	input.ArtifactPath = strings.TrimSpace(input.ArtifactPath)
	input.OutputDir = strings.TrimSpace(input.OutputDir)
	input.SecretEnvPath = strings.TrimSpace(input.SecretEnvPath)
	if input.ArtifactPath == "" {
		return Input{}, artifactcore.ErrArtifactPathRequired
	}
	if input.OutputDir == "" {
		return Input{}, artifactcore.ErrOutputDirRequired
	}
	return input, nil
}
