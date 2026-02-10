// Where: cli/internal/command/deploy_entry.go
// What: Deploy command entry and workflow execution.
// Why: Keep deploy command orchestration separate from input/detail helpers.
package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/env"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
)

// runDeploy executes the 'deploy' command.
func runDeploy(cli CLI, deps Dependencies, out io.Writer) int {
	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	emojiEnabled, err := resolveDeployEmojiEnabled(out, cli.Deploy)
	if err != nil {
		return exitWithError(out, err)
	}

	cmd, err := newDeployCommand(deps.Deploy, repoResolver, out, emojiEnabled)
	if err != nil {
		return exitWithError(out, err)
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		return exitWithError(out, err)
	}

	if err := cmd.Run(inputs, cli.Deploy); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type deployCommand struct {
	build         func(build.BuildRequest) error
	applyRuntime  func(state.Context) error
	ui            ui.UserInterface
	composeRunner compose.CommandRunner
	emojiEnabled  bool
}

func newDeployCommand(
	deployDeps DeployDeps,
	repoResolver func(string) (string, error),
	out io.Writer,
	emojiEnabled bool,
) (*deployCommand, error) {
	if deployDeps.Build == nil {
		return nil, errDeployBuilderNotConfigured
	}
	applyRuntimeEnv := deployDeps.ApplyRuntimeEnv
	if applyRuntimeEnv == nil {
		applyRuntimeEnv = func(ctx state.Context) error {
			return env.ApplyRuntimeEnv(ctx, repoResolver)
		}
	}
	ui := ui.NewDeployUI(out, emojiEnabled)
	return &deployCommand{
		build:         deployDeps.Build,
		applyRuntime:  applyRuntimeEnv,
		ui:            ui,
		composeRunner: compose.ExecRunner{},
		emojiEnabled:  emojiEnabled,
	}, nil
}

func (c *deployCommand) Run(inputs deployInputs, flags DeployCmd) error {
	tag := resolveBrandTag()
	imagePrewarm, err := normalizeImagePrewarm(flags.ImagePrewarm)
	if err != nil {
		return err
	}
	if flags.NoDeps && flags.WithDeps {
		return errors.New("deploy: --no-deps and --with-deps cannot be used together")
	}
	noDeps := true
	if flags.WithDeps {
		noDeps = false
	}
	if len(inputs.Templates) == 0 {
		return errTemplatePathRequired
	}
	workflow := deploy.NewDeployWorkflow(c.build, c.applyRuntime, c.ui, c.composeRunner)
	templateCount := len(inputs.Templates)
	for idx, tpl := range inputs.Templates {
		buildOnly := flags.BuildOnly || (templateCount > 1 && idx < templateCount-1)
		if c.ui != nil {
			title := "Deploy plan"
			if templateCount > 1 {
				title = fmt.Sprintf("Deploy plan (%d/%d)", idx+1, templateCount)
			}
			outputSummary := domaincfg.ResolveOutputSummary(tpl.TemplatePath, tpl.OutputDir, inputs.Env)
			composeFiles := "auto"
			if len(inputs.ComposeFiles) > 0 {
				composeFiles = strings.Join(inputs.ComposeFiles, ", ")
			}
			rows := []ui.KeyValue{
				{Key: "Template", Value: tpl.TemplatePath},
				{Key: "Env", Value: inputs.Env},
				{Key: "Mode", Value: inputs.Mode},
				{Key: "Project", Value: inputs.Project},
				{Key: "Output", Value: outputSummary},
				{Key: "BuildOnly", Value: buildOnly},
				{Key: "ImagePrewarm", Value: imagePrewarm},
				{Key: "ComposeFiles", Value: composeFiles},
			}
			c.ui.Block("🧭", title, rows)
		}
		ctx := state.Context{
			ProjectDir:     inputs.ProjectDir,
			TemplatePath:   tpl.TemplatePath,
			OutputDir:      tpl.OutputDir,
			Env:            inputs.Env,
			Mode:           inputs.Mode,
			ComposeProject: inputs.Project,
		}
		request := deploy.Request{
			Context:        ctx,
			Env:            inputs.Env,
			Mode:           inputs.Mode,
			TemplatePath:   tpl.TemplatePath,
			OutputDir:      tpl.OutputDir,
			Parameters:     tpl.Parameters,
			Tag:            tag,
			NoCache:        flags.NoCache,
			NoDeps:         noDeps,
			Verbose:        flags.Verbose,
			ComposeFiles:   inputs.ComposeFiles,
			BuildOnly:      buildOnly,
			BundleManifest: flags.Bundle,
			ImagePrewarm:   imagePrewarm,
			Emoji:          c.emojiEnabled,
		}
		if err := workflow.Run(request); err != nil {
			return fmt.Errorf("deploy workflow (%s): %w", tpl.TemplatePath, err)
		}
	}
	return nil
}

func normalizeImagePrewarm(value string) (string, error) {
	return deploy.NormalizeImagePrewarmMode(value)
}

func resolveDeployEmojiEnabled(out io.Writer, flags DeployCmd) (bool, error) {
	if flags.Emoji && flags.NoEmoji {
		return false, errors.New("deploy: --emoji and --no-emoji cannot be used together")
	}
	if flags.Emoji {
		return true, nil
	}
	if flags.NoEmoji {
		return false, nil
	}
	if strings.TrimSpace(os.Getenv("NO_EMOJI")) != "" {
		return false, nil
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "dumb" {
		return false, nil
	}
	if file, ok := out.(*os.File); ok {
		return interaction.IsTerminal(file), nil
	}
	return false, nil
}
