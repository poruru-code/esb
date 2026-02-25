package deployops

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type Input struct {
	ArtifactPath          string
	RuntimeConfigDir      string
	RuntimeConfigTarget   RuntimeConfigTarget
	NoCache               bool
	Runner                CommandRunner
	RuntimeConfigResolver RuntimeConfigResolver
	StagingRootDir        string
}

var errRuntimeConfigTargetRequired = fmt.Errorf("runtime-config target is required")

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
	stagingDir, cleanup, err := createStagingDir(normalized.StagingRootDir)
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	defer cleanup()

	result, err := artifactcore.ExecuteApply(artifactcore.ApplyInput{
		ArtifactPath: normalized.ArtifactPath,
		OutputDir:    stagingDir,
	})
	if err != nil {
		return artifactcore.ApplyResult{}, err
	}
	if err := normalizeOutputFunctionImages(stagingDir, prepareResult.publishedFunctionImages); err != nil {
		return artifactcore.ApplyResult{}, err
	}
	if err := syncRuntimeConfig(stagingDir, normalized.RuntimeConfigTarget); err != nil {
		return artifactcore.ApplyResult{}, err
	}
	return result, nil
}

func normalizeInput(input Input) (Input, error) {
	input.ArtifactPath = strings.TrimSpace(input.ArtifactPath)
	input.RuntimeConfigDir = strings.TrimSpace(input.RuntimeConfigDir)
	input.RuntimeConfigTarget = input.RuntimeConfigTarget.normalized()
	input.StagingRootDir = strings.TrimSpace(input.StagingRootDir)
	if input.ArtifactPath == "" {
		return Input{}, artifactcore.ErrArtifactPathRequired
	}
	if input.RuntimeConfigTarget.isEmpty() && input.RuntimeConfigDir != "" {
		input.RuntimeConfigTarget.BindPath = input.RuntimeConfigDir
	}
	if input.RuntimeConfigTarget.isEmpty() {
		resolver := input.RuntimeConfigResolver
		if resolver == nil {
			resolver = newDockerRuntimeConfigResolver()
		}
		resolved, err := resolver.ResolveRuntimeConfigTarget()
		if err != nil {
			return Input{}, err
		}
		input.RuntimeConfigTarget = resolved.normalized()
	}
	if input.RuntimeConfigTarget.isEmpty() {
		return Input{}, errRuntimeConfigTargetRequired
	}
	return input, nil
}

func createStagingDir(root string) (string, func(), error) {
	root = strings.TrimSpace(root)
	if root != "" {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", nil, fmt.Errorf("create staging root directory: %w", err)
		}
	}
	stagingDir, err := os.MkdirTemp(root, "artifact-runtime-config-*")
	if err != nil {
		return "", nil, fmt.Errorf("create staging runtime-config directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}
	return stagingDir, cleanup, nil
}
