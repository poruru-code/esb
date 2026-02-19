// Where: cli/internal/command/artifact.go
// What: CLI adapter for artifact apply operations.
// Why: Reuse shared artifact core logic from esb command.
package command

import (
	"errors"
	"io"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func runArtifactGenerate(cli CLI, deps Dependencies, out io.Writer) int {
	generatedCLI := cli
	generatedCLI.Deploy = artifactGenerateToDeployFlags(cli.Artifact.Generate)
	overrides := deployRunOverrides{
		buildImages:    boolPtr(cli.Artifact.Generate.BuildImages),
		forceBuildOnly: true,
	}
	return runDeployWithOverrides(generatedCLI, deps, out, overrides)
}

func artifactGenerateToDeployFlags(cmd ArtifactGenerateCmd) DeployCmd {
	return DeployCmd{
		Mode:         cmd.Mode,
		Output:       cmd.Output,
		Manifest:     cmd.Manifest,
		Project:      cmd.Project,
		ComposeFiles: append([]string(nil), cmd.ComposeFiles...),
		ImageURI:     append([]string(nil), cmd.ImageURI...),
		ImageRuntime: append([]string(nil), cmd.ImageRuntime...),
		BuildOnly:    true,
		Bundle:       cmd.Bundle,
		NoCache:      cmd.NoCache,
		Verbose:      cmd.Verbose,
		Emoji:        cmd.Emoji,
		NoEmoji:      cmd.NoEmoji,
		Force:        cmd.Force,
		NoSave:       cmd.NoSave,
	}
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}

func runArtifactApply(cli CLI, _ Dependencies, out io.Writer) int {
	args := cli.Artifact.Apply
	req := artifactcore.NewApplyRequest(
		args.Artifact,
		args.OutputDir,
		args.SecretEnv,
		args.Strict,
		out,
	)
	if req.ArtifactPath == "" {
		return exitWithError(out, errors.New("artifact apply: --artifact is required"))
	}
	if req.OutputDir == "" {
		return exitWithError(out, errors.New("artifact apply: --out is required"))
	}
	if err := artifactcore.Apply(req); err != nil {
		return exitWithError(out, err)
	}
	legacyUI(out).Success("Artifact apply complete")
	return 0
}
