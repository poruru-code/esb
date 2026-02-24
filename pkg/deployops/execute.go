package deployops

import (
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type Input struct {
	ArtifactPath string
	OutputDir    string
	NoCache      bool
	Runner       CommandRunner
}

func Execute(input Input) (artifactcore.ApplyResult, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	prepareResult, err := prepareImagesWithResult(prepareImagesInput{
		ArtifactPath: normalized.ArtifactPath,
		NoCache:      normalized.NoCache,
		Runner:       normalized.Runner,
		EnsureBase:   true,
	})
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}

	result, err := artifactcore.ExecuteApply(artifactcore.ApplyInput{
		ArtifactPath: normalized.ArtifactPath,
		OutputDir:    normalized.OutputDir,
	})
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	if err := normalizeOutputFunctionImages(normalized.OutputDir, prepareResult.publishedFunctionImages); err != nil {
		return artifactcore.ApplyResult{}, err
	}
	return result, nil
}

func normalizeInput(input Input) (Input, error) {
	input.ArtifactPath = strings.TrimSpace(input.ArtifactPath)
	input.OutputDir = strings.TrimSpace(input.OutputDir)
	if input.ArtifactPath == "" {
		return Input{}, artifactcore.ErrArtifactPathRequired
	}
	if input.OutputDir == "" {
		return Input{}, artifactcore.ErrOutputDirRequired
	}
	return input, nil
}
