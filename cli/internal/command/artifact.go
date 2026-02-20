// Where: cli/internal/command/artifact.go
// What: CLI adapter for artifact apply operations.
// Why: Reuse shared artifact core logic from esb command.
package command

import (
	"io"

	"github.com/poruru-code/esb/pkg/deployops"
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
	result, err := deployops.Execute(deployops.Input{
		ArtifactPath:  args.Artifact,
		OutputDir:     args.OutputDir,
		SecretEnvPath: args.SecretEnv,
	})
	if err != nil {
		return exitWithError(out, err)
	}
	deployUI := legacyUI(out)
	for _, warning := range result.Warnings {
		deployUI.Warn(warning)
	}
	deployUI.Success("Artifact apply complete")
	return 0
}
