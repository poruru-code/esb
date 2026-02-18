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
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
)

// runDeploy executes the 'deploy' command.
func runDeploy(cli CLI, deps Dependencies, out io.Writer) int {
	emojiEnabled, err := resolveDeployEmojiEnabled(out, cli.Deploy)
	if err != nil {
		return exitWithError(out, err)
	}

	commandConfig, err := resolveDeployCommandConfig(deps.Deploy, out, emojiEnabled)
	if err != nil {
		return exitWithError(out, err)
	}
	cmd := newDeployCommand(commandConfig)

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
	workflow      deployWorkflowDeps
	emojiEnabled  bool
}

type deployWorkflowDeps struct {
	composeProvisioner deploy.ComposeProvisioner
	registryWaiter     deploy.RegistryWaiter
	dockerClient       deploy.DockerClientFactory
}

type deployCommandConfig struct {
	build         func(build.BuildRequest) error
	applyRuntime  func(state.Context) error
	ui            ui.UserInterface
	composeRunner compose.CommandRunner
	workflow      deployWorkflowDeps
	emojiEnabled  bool
}

type deployRuntimeComponent struct {
	applyRuntime   func(state.Context) error
	registryWaiter deploy.RegistryWaiter
	dockerClient   deploy.DockerClientFactory
}

type deployProvisionComponent struct {
	ui                 ui.UserInterface
	composeRunner      compose.CommandRunner
	composeProvisioner deploy.ComposeProvisioner
}

func resolveDeployCommandConfig(
	deployDeps DeployDeps,
	out io.Writer,
	emojiEnabled bool,
) (deployCommandConfig, error) {
	buildFn, err := resolveDeployBuildComponent(deployDeps.Build)
	if err != nil {
		return deployCommandConfig{}, err
	}
	runtimeComponent, err := resolveDeployRuntimeComponent(deployDeps.Runtime)
	if err != nil {
		return deployCommandConfig{}, err
	}
	provisionComponent, err := resolveDeployProvisionComponent(deployDeps.Provision, out, emojiEnabled)
	if err != nil {
		return deployCommandConfig{}, err
	}
	return deployCommandConfig{
		build:         buildFn,
		applyRuntime:  runtimeComponent.applyRuntime,
		ui:            provisionComponent.ui,
		composeRunner: provisionComponent.composeRunner,
		workflow: deployWorkflowDeps{
			composeProvisioner: provisionComponent.composeProvisioner,
			registryWaiter:     runtimeComponent.registryWaiter,
			dockerClient:       runtimeComponent.dockerClient,
		},
		emojiEnabled: emojiEnabled,
	}, nil
}

func resolveDeployBuildComponent(buildDeps DeployBuildDeps) (func(build.BuildRequest) error, error) {
	if buildDeps.Build == nil {
		return nil, errDeployBuilderNotConfigured
	}
	return buildDeps.Build, nil
}

func resolveDeployRuntimeComponent(
	runtimeDeps DeployRuntimeDeps,
) (deployRuntimeComponent, error) {
	if runtimeDeps.ApplyRuntimeEnv == nil {
		return deployRuntimeComponent{}, errors.New("deploy: runtime env applier not configured")
	}
	return deployRuntimeComponent{
		applyRuntime:   runtimeDeps.ApplyRuntimeEnv,
		registryWaiter: deploy.RegistryWaiter(runtimeDeps.RegistryWaiter),
		dockerClient:   deploy.DockerClientFactory(runtimeDeps.DockerClient),
	}, nil
}

func resolveDeployProvisionComponent(
	provisionDeps DeployProvisionDeps,
	out io.Writer,
	emojiEnabled bool,
) (deployProvisionComponent, error) {
	if provisionDeps.ComposeRunner == nil {
		return deployProvisionComponent{}, errors.New("deploy: compose runner not configured")
	}
	deployUIFactory := provisionDeps.NewDeployUI
	if deployUIFactory == nil {
		return deployProvisionComponent{}, errors.New("deploy: deploy ui factory not configured")
	}
	deployUI := deployUIFactory(out, emojiEnabled)
	composeProvisioner := provisionDeps.ComposeProvisioner
	if composeProvisioner == nil && provisionDeps.ComposeProvisionerFactory != nil {
		composeProvisioner = provisionDeps.ComposeProvisionerFactory(deployUI)
	}
	if composeProvisioner == nil {
		return deployProvisionComponent{}, errors.New("deploy: compose provisioner not configured")
	}
	return deployProvisionComponent{
		ui:                 deployUI,
		composeRunner:      provisionDeps.ComposeRunner,
		composeProvisioner: composeProvisioner,
	}, nil
}

func newDeployCommand(config deployCommandConfig) *deployCommand {
	return &deployCommand{
		build:         config.build,
		applyRuntime:  config.applyRuntime,
		ui:            config.ui,
		composeRunner: config.composeRunner,
		workflow:      config.workflow,
		emojiEnabled:  config.emojiEnabled,
	}
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
	buildImages := true
	if flags.generateBuildImages != nil {
		buildImages = *flags.generateBuildImages
	}
	if len(inputs.Templates) == 0 {
		return errTemplatePathRequired
	}
	workflow := deploy.NewDeployWorkflow(c.build, c.applyRuntime, c.ui, c.composeRunner)
	if c.workflow.composeProvisioner != nil {
		workflow.ComposeProvisioner = c.workflow.composeProvisioner
	}
	if c.workflow.registryWaiter != nil {
		workflow.RegistryWaiter = c.workflow.registryWaiter
	}
	if c.workflow.dockerClient != nil {
		workflow.DockerClient = c.workflow.dockerClient
	}

	// Generate phase: build all templates and stage merged runtime-config.
	templateCount := len(inputs.Templates)
	for idx, tpl := range inputs.Templates {
		buildOnly := true
		if c.ui != nil {
			title := "Generate plan"
			if templateCount > 1 {
				title = fmt.Sprintf("Generate plan (%d/%d)", idx+1, templateCount)
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
				{Key: "BuildImages", Value: buildImages},
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
			ImageSources:   tpl.ImageSources,
			ImageRuntimes:  tpl.ImageRuntimes,
			Tag:            tag,
			NoCache:        flags.NoCache,
			NoDeps:         noDeps,
			Verbose:        flags.Verbose,
			ComposeFiles:   inputs.ComposeFiles,
			BuildOnly:      buildOnly,
			BuildImages:    boolPtr(buildImages),
			BundleManifest: flags.Bundle,
			ImagePrewarm:   imagePrewarm,
			Emoji:          c.emojiEnabled,
		}
		if err := workflow.Run(request); err != nil {
			return fmt.Errorf("deploy workflow (%s): %w", tpl.TemplatePath, err)
		}
	}

	// Materialize strict artifact manifest after all generate steps.
	manifestPath, err := writeDeployArtifactManifest(
		inputs,
		imagePrewarm,
		flags.Bundle,
		flags.Manifest,
	)
	if err != nil {
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	if c.ui != nil {
		c.ui.Info(fmt.Sprintf("Artifact manifest: %s", manifestPath))
	}
	if flags.BuildOnly {
		return nil
	}

	// Apply phase: no build, only artifact-driven apply + provisioner.
	applyTemplate := inputs.Templates[0]
	applyCtx := state.Context{
		ProjectDir:     inputs.ProjectDir,
		TemplatePath:   applyTemplate.TemplatePath,
		OutputDir:      applyTemplate.OutputDir,
		Env:            inputs.Env,
		Mode:           inputs.Mode,
		ComposeProject: inputs.Project,
	}
	applyReq := deploy.Request{
		Context:      applyCtx,
		Env:          inputs.Env,
		Mode:         inputs.Mode,
		TemplatePath: applyTemplate.TemplatePath,
		ArtifactPath: manifestPath,
		OutputDir:    applyTemplate.OutputDir,
		NoDeps:       noDeps,
		Verbose:      flags.Verbose,
		ComposeFiles: inputs.ComposeFiles,
		BuildOnly:    false,
		ImagePrewarm: imagePrewarm,
	}
	if err := workflow.Apply(applyReq); err != nil {
		return fmt.Errorf("deploy apply (%s): %w", applyTemplate.TemplatePath, err)
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
