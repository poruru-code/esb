// Where: cli/internal/command/deploy.go
// What: Deploy command implementation.
// Why: Orchestrate deploy operations in a testable way.
package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	runtimecfg "github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	domaintpl "github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/env"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/sam"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
	"github.com/poruru/edge-serverless-box/meta"
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
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "all", nil
	}
	switch trimmed {
	case "off", "all":
		return trimmed, nil
	default:
		return "", fmt.Errorf("deploy: invalid --image-prewarm value %q (use off|all)", value)
	}
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

type deployInputs struct {
	ProjectDir    string
	TargetStack   string
	Env           string
	EnvSource     string
	Mode          string
	Templates     []deployTemplateInput
	Project       string
	ProjectSource string
	ComposeFiles  []string
}

type deployTemplateInput struct {
	TemplatePath string
	OutputDir    string
	Parameters   map[string]string
}

type deployTargetStack struct {
	Name    string
	Project string
	Env     string
}

type envChoice struct {
	Value    string
	Source   string
	Explicit bool
}

const (
	templateHistoryLimit = 10
	templateManualOption = "Enter path..."
)

var (
	errDeployBuilderNotConfigured = errors.New("deploy: builder not configured")
	errTemplatePathRequired       = errors.New("template path is required")
	errEnvironmentRequired        = errors.New("environment is required")
	errModeRequired               = errors.New("mode is required")
	errMultipleRunningProjects    = errors.New("multiple running projects found (use --project)")
	errComposeProjectRequired     = errors.New("compose project is required")
	errTemplatePathEmpty          = errors.New("template path is empty")
	errTemplateNotFound           = errors.New("no template.yaml or template.yml found in directory")
	errParameterRequiresValue     = errors.New("parameter requires a value")
	errMultipleTemplateOutput     = errors.New("output directory cannot be used with multiple templates")
)

func resolveDeployInputs(cli CLI, deps Dependencies) (deployInputs, error) {
	isTTY := interaction.IsTerminal(os.Stdin)
	prompter := deps.Prompter
	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

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
			runningStacks, _ := discoverRunningDeployTargetStacks()
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
			prevEnv,
		)
		if err != nil {
			return deployInputs{}, err
		}

		inferredMode, inferredModeSource, modeInferErr := inferDeployModeFromProject(composeProject)

		previousTemplate := ""
		if len(last.Templates) > 0 {
			previousTemplate = last.Templates[0].TemplatePath
		}
		templatePaths, err := resolveDeployTemplates(cli.Template, isTTY, prompter, previousTemplate)
		if err != nil {
			return deployInputs{}, err
		}
		for range templatePaths[1:] {
			otherRoot, err := repoResolver("")
			if err != nil {
				return deployInputs{}, fmt.Errorf("resolve repo root: %w", err)
			}
			if otherRoot != repoRoot {
				return deployInputs{}, fmt.Errorf("template repo root mismatch: %s != %s", otherRoot, repoRoot)
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
				cli.Deploy.Force,
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
			fmt.Fprintf(os.Stderr, "Warning: failed to infer runtime mode: %v\n", modeInferErr)
		}

		var mode string
		switch {
		case inferredMode != "":
			if flagMode != "" && inferredMode != flagMode {
				fmt.Fprintf(
					os.Stderr,
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
			mode, err = resolveDeployMode("", isTTY, prompter, prevMode)
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
				var err error
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
			params, err := promptTemplateParameters(templatePath, isTTY, prompter, prevParams)
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
						fmt.Fprintf(os.Stderr, "Warning: failed to save deploy defaults: %v\n", err)
					}
				}
			}
			return inputs, nil
		}
		last = inputs
	}
}

func resolveDeployTemplate(
	value string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return normalizeTemplatePath(trimmed)
	}
	if !isTTY || prompter == nil {
		return "", errTemplatePathRequired
	}
	for {
		history := loadTemplateHistory()
		candidates := discoverTemplateCandidates()
		suggestions := domaintpl.BuildSuggestions(previous, history, candidates)
		defaultValue := ""
		if len(suggestions) > 0 {
			defaultValue = suggestions[0]
		}
		title := "Template path"
		if defaultValue != "" {
			title = fmt.Sprintf("Template path (default: %s)", defaultValue)
		}

		if len(history) > 0 || len(suggestions) > 0 {
			options := append([]string{}, suggestions...)
			options = append(options, templateManualOption)
			selected, err := prompter.Select(title, options)
			if err != nil {
				return "", fmt.Errorf("prompt template selection: %w", err)
			}
			if selected == templateManualOption {
				input, err := prompter.Input(title, suggestions)
				if err != nil {
					return "", fmt.Errorf("prompt template path: %w", err)
				}
				input = strings.TrimSpace(input)
				if input == "" {
					if defaultValue != "" {
						input = defaultValue
					} else if path, err := resolveTemplateFallback(previous, candidates); err == nil {
						return path, nil
					} else {
						fmt.Fprintln(os.Stderr, "Template path is required.")
						continue
					}
				}
				path, err := normalizeTemplatePath(input)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
					continue
				}
				return path, nil
			}
			path, err := normalizeTemplatePath(selected)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
				continue
			}
			return path, nil
		}

		input, err := prompter.Input(title, suggestions)
		if err != nil {
			return "", fmt.Errorf("prompt template path: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			if defaultValue != "" {
				input = defaultValue
			} else if path, err := resolveTemplateFallback(previous, candidates); err == nil {
				return path, nil
			} else {
				fmt.Fprintln(os.Stderr, "Template path is required.")
				continue
			}
		}
		path, err := normalizeTemplatePath(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid template path: %v\n", err)
			continue
		}
		return path, nil
	}
}

func resolveDeployTemplates(
	values []string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) ([]string, error) {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if v := strings.TrimSpace(value); v != "" {
			trimmed = append(trimmed, v)
		}
	}
	if len(trimmed) > 0 {
		out := make([]string, 0, len(trimmed))
		for _, value := range trimmed {
			path, err := normalizeTemplatePath(value)
			if err != nil {
				return nil, err
			}
			out = append(out, path)
		}
		return out, nil
	}
	path, err := resolveDeployTemplate("", isTTY, prompter, previous)
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func resolveDeployEnv(
	value string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (envChoice, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return envChoice{Value: trimmed, Source: "flag", Explicit: true}, nil
	}
	if !isTTY || prompter == nil {
		return envChoice{}, errEnvironmentRequired
	}
	defaultValue := strings.TrimSpace(previous)
	if defaultValue == "" {
		defaultValue = "default"
	}
	title := fmt.Sprintf("Environment name (default: %s)", defaultValue)
	input, err := prompter.Input(title, []string{defaultValue})
	if err != nil {
		return envChoice{}, fmt.Errorf("prompt environment: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return envChoice{Value: defaultValue, Source: "default", Explicit: false}, nil
	}
	return envChoice{Value: input, Source: "prompt", Explicit: true}, nil
}

func resolveDeployEnvFromStack(
	envValue string,
	stack deployTargetStack,
	composeProject string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (envChoice, error) {
	if trimmed := strings.TrimSpace(envValue); trimmed != "" {
		return envChoice{Value: trimmed, Source: "flag", Explicit: true}, nil
	}
	if env := strings.TrimSpace(stack.Env); env != "" {
		return envChoice{Value: env, Source: "stack", Explicit: false}, nil
	}
	if project := strings.TrimSpace(composeProject); project != "" {
		if inferred, err := inferEnvFromProject(project, ""); err == nil && strings.TrimSpace(inferred.Env) != "" {
			return envChoice{Value: inferred.Env, Source: inferred.Source, Explicit: false}, nil
		}
	}
	return resolveDeployEnv("", isTTY, prompter, previous)
}

func resolveDeployMode(
	value string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed != "" {
		normalized, err := runtimecfg.NormalizeMode(trimmed)
		if err != nil {
			return "", fmt.Errorf("normalize mode: %w", err)
		}
		return normalized, nil
	}
	if !isTTY || prompter == nil {
		return "", errModeRequired
	}
	defaultValue := strings.TrimSpace(strings.ToLower(previous))
	if defaultValue == "" {
		defaultValue = "docker"
	}
	for {
		options := []string{defaultValue}
		for _, opt := range []string{"docker", "containerd"} {
			if opt == defaultValue {
				continue
			}
			options = append(options, opt)
		}
		title := fmt.Sprintf("Runtime mode (default: %s)", defaultValue)
		selected, err := prompter.Select(title, options)
		if err != nil {
			return "", fmt.Errorf("prompt runtime mode: %w", err)
		}
		selected = strings.TrimSpace(strings.ToLower(selected))
		if selected == "" {
			fmt.Fprintln(os.Stderr, "Runtime mode is required.")
			continue
		}
		normalized, err := runtimecfg.NormalizeMode(selected)
		if err != nil {
			return "", fmt.Errorf("normalize mode: %w", err)
		}
		return normalized, nil
	}
}

func normalizeComposeFiles(files []string, baseDir string) []string {
	if len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed == "" {
			continue
		}
		path := trimmed
		if !filepath.IsAbs(path) && strings.TrimSpace(baseDir) != "" {
			path = filepath.Join(baseDir, path)
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		out = append(out, path)
		seen[path] = struct{}{}
	}
	return out
}

func resolveDeployOutput(
	value string,
	templatePath string,
	env string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	_ = templatePath
	_ = env

	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, nil
	}
	if !isTTY || prompter == nil {
		return "", nil
	}
	if prev := strings.TrimSpace(previous); prev != "" {
		return prev, nil
	}
	return "", nil
}

func deriveMultiTemplateOutputDir(templatePath string, counts map[string]int) string {
	base := strings.TrimSpace(filepath.Base(templatePath))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.TrimSpace(stem)
	if stem == "" {
		stem = "template"
	}
	count := counts[stem]
	counts[stem] = count + 1
	if count > 0 {
		stem = fmt.Sprintf("%s-%d", stem, count+1)
	}
	return filepath.Join(meta.OutputDir, stem)
}

func confirmDeployInputs(inputs deployInputs, isTTY bool, prompter interaction.Prompter) (bool, error) {
	if !isTTY || prompter == nil {
		return true, nil
	}

	stackLine := ""
	if stack := strings.TrimSpace(inputs.TargetStack); stack != "" {
		stackLine = fmt.Sprintf("Target Stack: %s", stack)
	}
	projectLine := fmt.Sprintf("Project: %s", inputs.Project)
	if strings.TrimSpace(inputs.ProjectSource) != "" {
		projectLine = fmt.Sprintf("Project: %s (%s)", inputs.Project, inputs.ProjectSource)
	}
	envLine := fmt.Sprintf("Env: %s", inputs.Env)
	if strings.TrimSpace(inputs.EnvSource) != "" {
		envLine = fmt.Sprintf("Env: %s (%s)", inputs.Env, inputs.EnvSource)
	}
	summaryLines := make([]string, 0, 4)
	if stackLine != "" {
		summaryLines = append(summaryLines, stackLine)
	}
	summaryLines = append(summaryLines,
		projectLine,
		envLine,
		fmt.Sprintf("Mode: %s", inputs.Mode),
	)
	if len(inputs.Templates) == 1 {
		summaryLines = appendTemplateSummaryLines(summaryLines, inputs.Templates[0], inputs.Env, inputs.Project)
	} else if len(inputs.Templates) > 1 {
		summaryLines = append(summaryLines, fmt.Sprintf("Templates: %d", len(inputs.Templates)))
		for _, tpl := range inputs.Templates {
			summaryLines = appendTemplateSummaryLines(summaryLines, tpl, inputs.Env, inputs.Project)
		}
	}

	summary := "Review inputs:\n" + strings.Join(summaryLines, "\n")

	choice, err := prompter.SelectValue(
		summary,
		[]interaction.SelectOption{
			{Label: "Proceed", Value: "proceed"},
			{Label: "Edit", Value: "edit"},
		},
	)
	if err != nil {
		return false, fmt.Errorf("prompt confirmation: %w", err)
	}
	return choice == "proceed", nil
}

func appendTemplateSummaryLines(
	lines []string,
	tpl deployTemplateInput,
	envName string,
	project string,
) []string {
	output := domaincfg.ResolveOutputSummary(tpl.TemplatePath, tpl.OutputDir, envName)
	templateBase := filepath.Dir(tpl.TemplatePath)
	stagingDir := "<unresolved>"
	if dir, err := staging.ConfigDir(tpl.TemplatePath, project, envName); err == nil {
		stagingDir = dir
	}
	lines = append(lines,
		fmt.Sprintf("Template: %s", tpl.TemplatePath),
		fmt.Sprintf("Template base: %s", templateBase),
		fmt.Sprintf("Output: %s", output),
		fmt.Sprintf("Staging config: %s", stagingDir),
	)
	if len(tpl.Parameters) == 0 {
		return lines
	}
	keys := make([]string, 0, len(tpl.Parameters))
	for key := range tpl.Parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines = append(lines, "Parameters:")
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %s = %s", key, tpl.Parameters[key]))
	}
	return lines
}

// storedDeployDefaults mirrors storedBuildDefaults for deploy.
type storedDeployDefaults struct {
	Env       string
	Mode      string
	OutputDir string
	Params    map[string]string
}

func loadTemplateHistory() []string {
	startDir, err := os.Getwd()
	if err != nil {
		return nil
	}
	repoRoot, err := config.ResolveRepoRoot(startDir)
	if err != nil {
		return nil
	}
	cfgPath, err := config.ProjectConfigPath(repoRoot)
	if err != nil {
		return nil
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		return nil
	}

	history := make([]string, 0, len(cfg.RecentTemplates))
	seen := map[string]struct{}{}
	for _, entry := range cfg.RecentTemplates {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		if _, err := os.Stat(trimmed); err != nil {
			continue
		}
		history = append(history, trimmed)
		seen[trimmed] = struct{}{}
		if len(history) >= templateHistoryLimit {
			break
		}
	}
	return history
}

func resolveTemplateFallback(previous string, candidates []string) (string, error) {
	if strings.TrimSpace(previous) != "" {
		return normalizeTemplatePath(previous)
	}
	if len(candidates) > 0 {
		return normalizeTemplatePath(candidates[0])
	}
	return normalizeTemplatePath(".")
}

func loadDeployDefaults(projectRoot, templatePath string) storedDeployDefaults {
	cfgPath, err := config.ProjectConfigPath(projectRoot)
	if err != nil || templatePath == "" {
		return storedDeployDefaults{}
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		return storedDeployDefaults{}
	}
	// Use BuildDefaults for now - can be separated later if needed
	if cfg.BuildDefaults == nil {
		return storedDeployDefaults{}
	}
	entry, ok := cfg.BuildDefaults[templatePath]
	if !ok {
		return storedDeployDefaults{}
	}
	return storedDeployDefaults{
		Env:       entry.Env,
		Mode:      entry.Mode,
		OutputDir: entry.OutputDir,
		Params:    cloneParams(entry.Params),
	}
}

func saveDeployDefaults(projectRoot string, template deployTemplateInput, inputs deployInputs) error {
	if strings.TrimSpace(template.TemplatePath) == "" {
		return nil
	}
	cfgPath, err := config.ProjectConfigPath(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project config path: %w", err)
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		cfg = config.DefaultGlobalConfig()
	}
	if cfg.BuildDefaults == nil {
		cfg.BuildDefaults = map[string]config.BuildDefaults{}
	}
	cfg.BuildDefaults[template.TemplatePath] = config.BuildDefaults{
		Env:       inputs.Env,
		Mode:      inputs.Mode,
		OutputDir: template.OutputDir,
		Params:    cloneParams(template.Parameters),
	}
	cfg.RecentTemplates = domaintpl.UpdateHistory(cfg.RecentTemplates, template.TemplatePath, templateHistoryLimit)
	if err := config.SaveGlobalConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}
	return nil
}

// Helper functions from deleted build.go

func resolveBrandTag() string {
	// Use brand-prefixed environment variable (e.g., ESB_TAG)
	key, err := envutil.HostEnvKey(constants.HostSuffixTag)
	if err == nil {
		tag := os.Getenv(key)
		if tag != "" {
			return tag
		}
	}
	return "latest"
}

func normalizeTemplatePath(path string) (string, error) {
	if path == "" {
		return "", errTemplatePathEmpty
	}

	// Expand ~ to home directory
	expanded, err := expandHomePath(path)
	if err != nil {
		return "", fmt.Errorf("expand home path: %w", err)
	}

	cleaned := filepath.Clean(expanded)
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) && !filepath.IsAbs(cleaned) {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				if repoRoot, repoErr := config.ResolveRepoRoot(cwd); repoErr == nil {
					candidate := filepath.Join(repoRoot, cleaned)
					if altInfo, altErr := os.Stat(candidate); altErr == nil {
						cleaned = candidate
						info = altInfo
						err = nil
					}
				}
			}
		}
		if err != nil {
			return "", fmt.Errorf("stat template path: %w", err)
		}
	}

	// If it's a file, return its absolute path
	if !info.IsDir() {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("resolve template path: %w", err)
		}
		return abs, nil
	}

	// If it's a directory, look for template.yaml or template.yml
	for _, name := range []string{"template.yaml", "template.yml"} {
		candidate := filepath.Join(cleaned, name)
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return "", fmt.Errorf("resolve template path: %w", err)
			}
			return abs, nil
		}
	}

	// No template file found in directory
	return "", fmt.Errorf("%w: %s", errTemplateNotFound, cleaned)
}

// expandHomePath expands ~ to the user's home directory.
func expandHomePath(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	if len(path) == 1 || path[1] == '/' || path[1] == filepath.Separator {
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil // ~username not supported
}

func discoverTemplateCandidates() []string {
	candidates := []string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		return candidates
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".git" || name == meta.OutputDir || name == "node_modules" || name == ".venv" {
			continue
		}
		if name == "__pycache__" || name == ".pytest_cache" || name == ".mypy_cache" {
			continue
		}
		if !hasTemplateFile(name) {
			continue
		}
		candidates = append(candidates, name)
	}
	return candidates
}

func hasTemplateFile(dir string) bool {
	for _, name := range []string{"template.yaml", "template.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func promptTemplateParameters(templatePath string, isTTY bool, prompter interaction.Prompter, previous map[string]string) (map[string]string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return map[string]string{}, fmt.Errorf("read template: %w", err)
	}

	data, err := sam.DecodeYAML(string(content))
	if err != nil {
		return map[string]string{}, fmt.Errorf("decode template: %w", err)
	}

	params := extractSAMParameters(data)
	if len(params) == 0 {
		return map[string]string{}, nil
	}

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	values := make(map[string]string, len(params))
	for _, name := range names {
		param := params[name]
		hasDefault := param.Default != nil
		defaultStr := ""
		if hasDefault {
			defaultStr = fmt.Sprint(param.Default)
		}
		prevValue := ""
		if previous != nil {
			prevValue = strings.TrimSpace(previous[name])
		}

		if !isTTY || prompter == nil {
			if hasDefault {
				values[name] = defaultStr
				continue
			}
			if prevValue != "" {
				values[name] = prevValue
				continue
			}
			return nil, fmt.Errorf("%w: %s", errParameterRequiresValue, name)
		}

		label := name
		if param.Description != "" {
			label = fmt.Sprintf("%s (%s)", name, param.Description)
		}

		var title string
		switch {
		case hasDefault:
			displayDefault := defaultStr
			if displayDefault == "" {
				displayDefault = "''"
			}
			title = fmt.Sprintf("%s [Default: %s]", label, displayDefault)
		case prevValue != "":
			title = fmt.Sprintf("%s [Previous: %s]", label, prevValue)
		default:
			title = fmt.Sprintf("%s [Required]", label)
		}

		suggestions := []string{}
		if prevValue != "" {
			suggestions = append(suggestions, prevValue)
		}
		for {
			input, err := prompter.Input(title, suggestions)
			if err != nil {
				return nil, fmt.Errorf("prompt parameter %s: %w", name, err)
			}
			input = strings.TrimSpace(input)
			if input == "" && hasDefault {
				input = defaultStr
			}
			if input == "" && prevValue != "" {
				input = prevValue
			}
			if input == "" && !hasDefault {
				if strings.EqualFold(param.Type, "String") {
					values[name] = ""
					break
				}
				fmt.Fprintf(os.Stderr, "Parameter %q is required.\n", name)
				continue
			}
			values[name] = input
			break
		}
	}

	return values, nil
}

// samParameter represents a SAM template parameter definition.
type samParameter struct {
	Type        string
	Description string
	Default     any
}

// extractSAMParameters extracts parameter definitions from SAM template data.
func extractSAMParameters(data map[string]any) map[string]samParameter {
	result := make(map[string]samParameter)
	params := asMap(data["Parameters"])
	if params == nil {
		return result
	}

	for name, val := range params {
		m := asMap(val)
		if m == nil {
			continue
		}

		param := samParameter{}
		if t, ok := m["Type"].(string); ok {
			param.Type = t
		}
		if d, ok := m["Description"].(string); ok {
			param.Description = d
		}
		param.Default = m["Default"]
		result[name] = param
	}

	return result
}

// asMap converts an interface to a map[string]any.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if m, ok := v.(map[any]any); ok {
		result := make(map[string]any, len(m))
		for k, v := range m {
			if sk, ok := k.(string); ok {
				result[sk] = v
			}
		}
		return result
	}
	return nil
}

func cloneParams(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
