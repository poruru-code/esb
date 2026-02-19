package deployops

import (
	"strings"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

type Input struct {
	ArtifactPath  string
	OutputDir     string
	SecretEnvPath string
	Strict        bool
	NoCache       bool
	Runner        CommandRunner
}

func Execute(input Input) (artifactcore.ApplyResult, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	if err := prepareImages(prepareImagesInput{
		ArtifactPath: normalized.ArtifactPath,
		NoCache:      normalized.NoCache,
		Runner:       normalized.Runner,
	}); err != nil {
		return artifactcore.ApplyResult{}, err
	}
	return artifactcore.ExecuteApply(artifactcore.ApplyInput{
		ArtifactPath:  normalized.ArtifactPath,
		OutputDir:     normalized.OutputDir,
		SecretEnvPath: normalized.SecretEnvPath,
		Strict:        normalized.Strict,
	})
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
