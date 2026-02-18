// Where: cli/internal/command/artifact.go
// What: CLI adapter for artifact apply operations.
// Why: Reuse tools/artifactctl Go engine from esb command.
package command

import (
	"errors"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/engine"
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
		ImagePrewarm: cmd.ImagePrewarm,
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
	artifactPath := strings.TrimSpace(args.Artifact)
	if artifactPath == "" {
		return exitWithError(out, errors.New("artifact apply: --artifact is required"))
	}
	outputDir := strings.TrimSpace(args.OutputDir)
	if outputDir == "" {
		return exitWithError(out, errors.New("artifact apply: --out is required"))
	}
	req := engine.ApplyRequest{
		ArtifactPath:  artifactPath,
		OutputDir:     outputDir,
		SecretEnvPath: strings.TrimSpace(args.SecretEnv),
		Strict:        args.Strict,
		WarningWriter: out,
	}
	if err := engine.Apply(req); err != nil {
		return exitWithError(out, err)
	}
	legacyUI(out).Success("Artifact apply complete")
	return 0
}
