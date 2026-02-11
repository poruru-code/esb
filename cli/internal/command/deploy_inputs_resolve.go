// Where: cli/internal/command/deploy_inputs_resolve.go
// What: Deploy input orchestration, env resolution, and mode resolution.
// Why: Keep the main input flow isolated from path/output helper details.
package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	runtimecfg "github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
)

func resolveDeployInputs(cli CLI, deps Dependencies) (deployInputs, error) {
	isTTY := interaction.IsTerminal(os.Stdin)
	prompter := deps.Prompter
	errOut := resolveErrWriter(deps.ErrOut)
	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}
	runtimeResolver := deps.Deploy.Runtime.RuntimeEnvResolver
	if runtimeResolver == nil {
		runtimeResolver = runtimeinfra.NewEnvResolver()
	}
	dockerClientFactory := deps.Deploy.Runtime.DockerClient

	var last deployInputs
	for {
		repoRoot, err := repoResolver("")
		if err != nil {
			return deployInputs{}, fmt.Errorf("resolve repo root: %w", err)
		}
		if err := config.EnsureProjectConfig(repoRoot); err != nil {
			return deployInputs{}, err
		}
		prevEnv := strings.TrimSpace(last.Env)

		projectValueSource := ""
		projectValue := strings.TrimSpace(cli.Deploy.Project)
		if projectValue != "" {
			projectValueSource = "flag"
		}
		if projectValue == "" {
			if envProject := strings.TrimSpace(os.Getenv(constants.EnvProjectName)); envProject != "" {
				projectValue = envProject
				projectValueSource = "env"
			}
		}
		if projectValue == "" {
			if hostProject, err := envutil.GetHostEnv(constants.HostSuffixProject); err == nil {
				if trimmed := strings.TrimSpace(hostProject); trimmed != "" {
					projectValue = trimmed
					projectValueSource = "host"
				}
			}
		}
		projectExplicit := projectValueSource != ""

		var selectedStack deployTargetStack
		if !projectExplicit {
			runningStacks, stackDiscoverErr := discoverRunningDeployTargetStacks(dockerClientFactory)
			if stackDiscoverErr != nil {
				writeWarningf(errOut, "Warning: failed to discover running stacks: %v\n", stackDiscoverErr)
			}
			selectedStack, err = resolveDeployTargetStack(runningStacks, isTTY, prompter)
			if err != nil {
				return deployInputs{}, err
			}
		}

		composeProject := ""
		projectSource := ""
		switch {
		case projectExplicit:
			composeProject = projectValue
			projectSource = projectValueSource
		case strings.TrimSpace(selectedStack.Project) != "":
			composeProject = selectedStack.Project
			projectSource = "stack"
		default:
			defaultEnv := strings.TrimSpace(cli.EnvFlag)
			if defaultEnv == "" {
				defaultEnv = strings.TrimSpace(selectedStack.Env)
			}
			if defaultEnv == "" {
				defaultEnv = prevEnv
			}
			composeProject = defaultDeployProject(defaultEnv)
			projectSource = "default"
		}
		if strings.TrimSpace(composeProject) == "" {
			return deployInputs{}, errComposeProjectRequired
		}

		selectedEnv, err := resolveDeployEnvFromStack(
			cli.EnvFlag,
			selectedStack,
			composeProject,
			isTTY,
			prompter,
			runtimeResolver,
			prevEnv,
		)
		if err != nil {
			return deployInputs{}, err
		}

		inferredMode, inferredModeSource, modeInferErr := inferDeployModeFromProject(composeProject, dockerClientFactory)

		previousTemplate := ""
		if len(last.Templates) > 0 {
			previousTemplate = last.Templates[0].TemplatePath
		}
		templatePaths, err := resolveDeployTemplates(cli.Template, isTTY, prompter, previousTemplate, errOut)
		if err != nil {
			return deployInputs{}, err
		}
		templateRoot, err := repoResolver(filepath.Dir(templatePaths[0]))
		if err != nil {
			return deployInputs{}, fmt.Errorf("resolve repo root: %w", err)
		}
		if templateRoot != repoRoot {
			return deployInputs{}, fmt.Errorf("template repo root mismatch: %s != %s", templateRoot, repoRoot)
		}
		for _, otherTemplate := range templatePaths[1:] {
			otherRoot, err := repoResolver(filepath.Dir(otherTemplate))
			if err != nil {
				return deployInputs{}, fmt.Errorf("resolve repo root: %w", err)
			}
			if otherRoot != templateRoot {
				return deployInputs{}, fmt.Errorf("template repo root mismatch: %s != %s", otherRoot, templateRoot)
			}
		}
		stored := loadDeployDefaults(repoRoot, templatePaths[0])
		if inferredMode != "" {
			selectedEnv, err = reconcileEnvWithRuntime(
				selectedEnv,
				composeProject,
				templatePaths[0],
				isTTY,
				prompter,
				runtimeResolver,
				cli.Deploy.Force,
				errOut,
			)
			if err != nil {
				return deployInputs{}, err
			}
		}
		if strings.TrimSpace(selectedEnv.Value) == "" {
			chosen, err := resolveDeployEnv("", isTTY, prompter, prevEnv)
			if err != nil {
				return deployInputs{}, err
			}
			selectedEnv = chosen
		}
		envChanged := selectedEnv.Value != prevEnv

		prevMode := strings.TrimSpace(last.Mode)
		if prevMode == "" {
			prevMode = strings.TrimSpace(stored.Mode)
		}
		flagMode := strings.TrimSpace(cli.Deploy.Mode)
		if flagMode != "" {
			normalized, err := runtimecfg.NormalizeMode(flagMode)
			if err != nil {
				return deployInputs{}, fmt.Errorf("normalize mode: %w", err)
			}
			flagMode = normalized
		}

		if modeInferErr != nil {
			writeWarningf(errOut, "Warning: failed to infer runtime mode: %v\n", modeInferErr)
		}

		var mode string
		switch {
		case inferredMode != "":
			if flagMode != "" && inferredMode != flagMode {
				writeWarningf(
					errOut,
					"Warning: running project uses %s mode; ignoring --mode %s (source: %s)\n",
					inferredMode,
					flagMode,
					inferredModeSource,
				)
			}
			mode = inferredMode
		case flagMode != "":
			mode = flagMode
		default:
			mode, err = resolveDeployMode("", isTTY, prompter, prevMode, errOut)
			if err != nil {
				return deployInputs{}, err
			}
		}

		if len(templatePaths) > 1 && strings.TrimSpace(cli.Deploy.Output) != "" {
			return deployInputs{}, errMultipleTemplateOutput
		}
		prevTemplates := map[string]deployTemplateInput{}
		for _, tpl := range last.Templates {
			prevTemplates[tpl.TemplatePath] = tpl
		}
		outputKeyCounts := map[string]int{}
		templateInputs := make([]deployTemplateInput, 0, len(templatePaths))
		for _, templatePath := range templatePaths {
			storedTemplate := loadDeployDefaults(repoRoot, templatePath)
			outputDir := ""
			if len(templatePaths) == 1 {
				prevOutput := ""
				if prev, ok := prevTemplates[templatePath]; ok && strings.TrimSpace(prev.OutputDir) != "" {
					prevOutput = prev.OutputDir
				} else if strings.TrimSpace(storedTemplate.OutputDir) != "" {
					prevOutput = storedTemplate.OutputDir
				}
				if envChanged && strings.TrimSpace(cli.Deploy.Output) == "" {
					prevOutput = ""
				}
				outputDir, err = resolveDeployOutput(
					cli.Deploy.Output,
					templatePath,
					selectedEnv.Value,
					isTTY,
					prompter,
					prevOutput,
				)
				if err != nil {
					return deployInputs{}, err
				}
			} else {
				outputDir = deriveMultiTemplateOutputDir(templatePath, outputKeyCounts)
			}

			prevParams := storedTemplate.Params
			if prev, ok := prevTemplates[templatePath]; ok && len(prev.Parameters) > 0 {
				prevParams = prev.Parameters
			}
			params, err := promptTemplateParameters(templatePath, isTTY, prompter, prevParams, errOut)
			if err != nil {
				return deployInputs{}, err
			}

			templateInputs = append(templateInputs, deployTemplateInput{
				TemplatePath: templatePath,
				OutputDir:    outputDir,
				Parameters:   params,
			})
		}

		projectDir := repoRoot
		composeFiles := normalizeComposeFiles(cli.Deploy.ComposeFiles, projectDir)

		inputs := deployInputs{
			ProjectDir:    projectDir,
			TargetStack:   selectedStack.Name,
			Env:           selectedEnv.Value,
			EnvSource:     selectedEnv.Source,
			Mode:          mode,
			Templates:     templateInputs,
			Project:       composeProject,
			ProjectSource: projectSource,
			ComposeFiles:  composeFiles,
		}

		confirmed, err := confirmDeployInputs(inputs, isTTY, prompter)
		if err != nil {
			return deployInputs{}, err
		}
		if confirmed {
			if !cli.Deploy.NoSave {
				for _, tpl := range templateInputs {
					if err := saveDeployDefaults(repoRoot, tpl, inputs); err != nil {
						writeWarningf(errOut, "Warning: failed to save deploy defaults: %v\n", err)
					}
				}
			}
			return inputs, nil
		}
		last = inputs
	}
}
