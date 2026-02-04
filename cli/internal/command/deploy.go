// Where: cli/internal/command/deploy.go
// What: Deploy command implementation.
// Why: Orchestrate deploy operations in a testable way.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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

	cmd, err := newDeployCommand(deps.Deploy, repoResolver, out)
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
}

func newDeployCommand(
	deployDeps DeployDeps,
	repoResolver func(string) (string, error),
	out io.Writer,
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
	ui := ui.NewLegacyUI(out)
	return &deployCommand{
		build:         deployDeps.Build,
		applyRuntime:  applyRuntimeEnv,
		ui:            ui,
		composeRunner: compose.ExecRunner{},
	}, nil
}

func (c *deployCommand) Run(inputs deployInputs, flags DeployCmd) error {
	tag := resolveBrandTag()
	if flags.NoDeps && flags.WithDeps {
		return errors.New("deploy: --no-deps and --with-deps cannot be used together")
	}
	noDeps := true
	if flags.WithDeps {
		noDeps = false
	}
	request := deploy.Request{
		Context:      inputs.Context,
		Env:          inputs.Env,
		Mode:         inputs.Mode,
		TemplatePath: inputs.TemplatePath,
		OutputDir:    inputs.OutputDir,
		Parameters:   inputs.Parameters,
		Tag:          tag,
		NoCache:      flags.NoCache,
		NoDeps:       noDeps,
		Verbose:      flags.Verbose,
		ComposeFiles: inputs.ComposeFiles,
	}
	if err := deploy.NewDeployWorkflow(c.build, c.applyRuntime, c.ui, c.composeRunner).Run(request); err != nil {
		return fmt.Errorf("deploy workflow: %w", err)
	}
	return nil
}

type deployInputs struct {
	Context       state.Context
	Env           string
	EnvSource     string
	Mode          string
	TemplatePath  string
	OutputDir     string
	Project       string
	ProjectSource string
	Parameters    map[string]string
	ComposeFiles  []string
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
)

func resolveDeployInputs(cli CLI, deps Dependencies) (deployInputs, error) {
	isTTY := interaction.IsTerminal(os.Stdin)
	prompter := deps.Prompter

	var last deployInputs
	for {
		templatePath, err := resolveDeployTemplate(cli.Template, isTTY, prompter, last.TemplatePath)
		if err != nil {
			return deployInputs{}, err
		}
		stored := loadDeployDefaults(templatePath)
		prevEnv := strings.TrimSpace(last.Env)
		if prevEnv == "" {
			prevEnv = strings.TrimSpace(stored.Env)
		}

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

		projectValue := strings.TrimSpace(cli.Deploy.Project)
		if projectValue == "" {
			projectValue = strings.TrimSpace(os.Getenv(constants.EnvProjectName))
		}
		if projectValue == "" {
			if hostProject, err := envutil.GetHostEnv(constants.HostSuffixProject); err == nil {
				projectValue = strings.TrimSpace(hostProject)
			}
		}
		runningProjects, _ := discoverRunningComposeProjects()
		hasRunning := len(runningProjects) > 0

		selectedEnv := envChoice{}
		if !hasRunning {
			chosen, err := resolveDeployEnv(cli.EnvFlag, isTTY, prompter, prevEnv)
			if err != nil {
				return deployInputs{}, err
			}
			selectedEnv = chosen
		} else if trimmed := strings.TrimSpace(cli.EnvFlag); trimmed != "" {
			selectedEnv = envChoice{Value: trimmed, Source: "flag", Explicit: true}
		}

		composeProject, projectSource, err := resolveDeployProject(
			projectValue,
			selectedEnv.Value,
			isTTY,
			prompter,
			runningProjects,
		)
		if err != nil {
			return deployInputs{}, err
		}

		if hasRunning {
			selectedEnv, err = reconcileEnvWithRuntime(
				selectedEnv,
				composeProject,
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

		var mode string
		switch {
		case hasRunning:
			inferredMode, source, err := inferDeployModeFromProject(composeProject)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to infer runtime mode: %v\n", err)
			}
			switch {
			case inferredMode != "":
				if flagMode != "" && inferredMode != flagMode {
					fmt.Fprintf(
						os.Stderr,
						"Warning: running project uses %s mode; ignoring --mode %s (source: %s)\n",
						inferredMode,
						flagMode,
						source,
					)
				}
				mode = inferredMode
			case flagMode != "":
				mode = flagMode
			default:
				mode = runtimecfg.FallbackMode(prevMode)
			}
		case flagMode != "":
			mode = flagMode
		default:
			var err error
			mode, err = resolveDeployMode("", isTTY, prompter, prevMode)
			if err != nil {
				return deployInputs{}, err
			}
		}

		prevOutput := strings.TrimSpace(last.OutputDir)
		if prevOutput == "" {
			prevOutput = strings.TrimSpace(stored.OutputDir)
		}
		if envChanged && strings.TrimSpace(cli.Deploy.Output) == "" {
			prevOutput = ""
		}
		outputDir, err := resolveDeployOutput(
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

		prevParams := last.Parameters
		if len(prevParams) == 0 {
			prevParams = stored.Params
		}
		params, err := promptTemplateParameters(templatePath, isTTY, prompter, prevParams)
		if err != nil {
			return deployInputs{}, err
		}

		projectDir, err := os.Getwd()
		if err != nil {
			return deployInputs{}, fmt.Errorf("get working directory: %w", err)
		}

		composeFiles := normalizeComposeFiles(cli.Deploy.ComposeFiles, projectDir)
		ctx := state.Context{
			ProjectDir:     projectDir,
			TemplatePath:   templatePath,
			OutputDir:      outputDir,
			Env:            selectedEnv.Value,
			Mode:           mode,
			ComposeProject: composeProject,
		}

		inputs := deployInputs{
			Context:       ctx,
			Env:           selectedEnv.Value,
			EnvSource:     selectedEnv.Source,
			Mode:          mode,
			TemplatePath:  templatePath,
			OutputDir:     outputDir,
			Project:       composeProject,
			ProjectSource: projectSource,
			Parameters:    params,
			ComposeFiles:  composeFiles,
		}

		confirmed, err := confirmDeployInputs(inputs, isTTY, prompter)
		if err != nil {
			return deployInputs{}, err
		}
		if confirmed {
			if !cli.Deploy.NoSave {
				if err := saveDeployDefaults(templatePath, inputs); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save deploy defaults: %v\n", err)
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
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, nil
	}
	if !isTTY || prompter == nil {
		return "", nil
	}

	defaultBase := meta.OutputDir
	defaultResolved := filepath.Join(filepath.Dir(templatePath), defaultBase, env)
	defaultValue := defaultResolved
	if prev := strings.TrimSpace(previous); prev != "" {
		defaultValue = prev
	}

	suggestions := []string{}
	if prev := strings.TrimSpace(previous); prev != "" {
		suggestions = append(suggestions, prev)
	}
	if defaultBase != "" && defaultBase != previous {
		suggestions = append(suggestions, defaultBase)
	}

	title := fmt.Sprintf("Output directory (default: %s)", defaultValue)
	input, err := prompter.Input(title, suggestions)
	if err != nil {
		return "", fmt.Errorf("prompt output dir: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		if strings.TrimSpace(previous) != "" {
			return previous, nil
		}
		return "", nil
	}
	cleaned := filepath.Clean(input)
	if filepath.Clean(defaultResolved) == cleaned {
		return defaultBase, nil
	}
	return cleaned, nil
}

func confirmDeployInputs(inputs deployInputs, isTTY bool, prompter interaction.Prompter) (bool, error) {
	if !isTTY || prompter == nil {
		return true, nil
	}

	output := domaincfg.ResolveOutputSummary(inputs.TemplatePath, inputs.OutputDir, inputs.Env)

	templateBase := filepath.Dir(inputs.TemplatePath)
	projectLine := fmt.Sprintf("Project: %s", inputs.Project)
	if strings.TrimSpace(inputs.ProjectSource) != "" {
		projectLine = fmt.Sprintf("Project: %s (%s)", inputs.Project, inputs.ProjectSource)
	}
	envLine := fmt.Sprintf("Env: %s", inputs.Env)
	if strings.TrimSpace(inputs.EnvSource) != "" {
		envLine = fmt.Sprintf("Env: %s (%s)", inputs.Env, inputs.EnvSource)
	}
	stagingDir := staging.ConfigDir(inputs.Project, inputs.Env)

	summaryLines := []string{
		fmt.Sprintf("Template: %s", inputs.TemplatePath),
		fmt.Sprintf("Template base: %s", templateBase),
		projectLine,
		envLine,
		fmt.Sprintf("Mode: %s", inputs.Mode),
		fmt.Sprintf("Output: %s", output),
		fmt.Sprintf("Staging config: %s", stagingDir),
	}
	if len(inputs.Parameters) > 0 {
		keys := make([]string, 0, len(inputs.Parameters))
		for k := range inputs.Parameters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		paramLines := make([]string, 0, len(keys)+1)
		paramLines = append(paramLines, "Parameters:")
		for _, key := range keys {
			paramLines = append(paramLines, fmt.Sprintf("  %s = %s", key, inputs.Parameters[key]))
		}
		summaryLines = append(summaryLines, paramLines...)
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

func resolveDeployProject(
	value string,
	env string,
	isTTY bool,
	prompter interaction.Prompter,
	running []string,
) (string, string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, "flag", nil
	}

	defaultProject := defaultDeployProject(env)
	if len(running) == 1 && (!isTTY || prompter == nil) {
		return running[0], "running", nil
	}
	if len(running) > 1 && (!isTTY || prompter == nil) {
		return "", "", fmt.Errorf("%w: %s", errMultipleRunningProjects, strings.Join(running, ", "))
	}
	if len(running) > 0 && isTTY && prompter != nil {
		options := append([]string{}, running...)
		title := "Compose project (running)"
		selected, err := prompter.Select(title, options)
		if err != nil {
			return "", "", fmt.Errorf("prompt compose project: %w", err)
		}
		selected = strings.TrimSpace(selected)
		if selected != "" {
			return selected, "running", nil
		}
	}
	if defaultProject == "" {
		return "", "", errComposeProjectRequired
	}
	return defaultProject, "default", nil
}

func defaultDeployProject(env string) string {
	brandName := strings.ToLower(strings.TrimSpace(os.Getenv("CLI_CMD")))
	if brandName == "" {
		brandName = meta.Slug
	}
	envName := strings.ToLower(strings.TrimSpace(env))
	if envName == "" {
		envName = "default"
	}
	return fmt.Sprintf("%s-%s", brandName, envName)
}

func discoverRunningComposeProjects() ([]string, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()
	containers, err := client.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	allowedServices := map[string]struct{}{
		"gateway":      {},
		"agent":        {},
		"runtime-node": {},
		"registry":     {},
		"database":     {},
		"s3-storage":   {},
		"victorialogs": {},
	}
	projects := make(map[string]struct{})
	for _, ctr := range containers {
		if ctr.Labels == nil {
			continue
		}
		project := strings.TrimSpace(ctr.Labels[compose.ComposeProjectLabel])
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		if project == "" {
			continue
		}
		if _, ok := allowedServices[service]; !ok {
			continue
		}
		projects[project] = struct{}{}
	}

	if len(projects) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(projects))
	for project := range projects {
		names = append(names, project)
	}
	sort.Strings(names)
	return names, nil
}

func inferDeployModeFromProject(composeProject string) (string, string, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return "", "", nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return "", "", fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()

	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, trimmed))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return "", "", fmt.Errorf("list containers: %w", err)
	}
	infos := containerInfos(containers)
	if mode := runtimecfg.InferModeFromContainers(infos, true); mode != "" {
		return mode, "running_services", nil
	}
	if mode := runtimecfg.InferModeFromContainers(infos, false); mode != "" {
		return mode, "services", nil
	}

	result, err := compose.ResolveComposeFilesFromProject(ctx, client, trimmed)
	if err == nil {
		if mode := runtimecfg.InferModeFromComposeFiles(result.Files); mode != "" {
			return mode, "config_files", nil
		}
	} else {
		return "", "", fmt.Errorf("resolve compose files: %w", err)
	}
	return "", "", nil
}

func containerInfos(containers []container.Summary) []runtimecfg.ContainerInfo {
	if len(containers) == 0 {
		return nil
	}
	out := make([]runtimecfg.ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		service := ""
		if ctr.Labels != nil {
			service = strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		}
		out = append(out, runtimecfg.ContainerInfo{
			Service: service,
			State:   strings.TrimSpace(ctr.State),
		})
	}
	return out
}

// storedDeployDefaults mirrors storedBuildDefaults for deploy.
type storedDeployDefaults struct {
	Env       string
	Mode      string
	OutputDir string
	Params    map[string]string
}

func loadTemplateHistory() []string {
	cfgPath, err := config.GlobalConfigPath()
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

func loadDeployDefaults(templatePath string) storedDeployDefaults {
	cfgPath, err := config.GlobalConfigPath()
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

func saveDeployDefaults(templatePath string, inputs deployInputs) error {
	if templatePath == "" {
		return nil
	}
	cfgPath, err := config.GlobalConfigPath()
	if err != nil {
		return fmt.Errorf("resolve global config path: %w", err)
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		cfg = config.DefaultGlobalConfig()
	}
	if cfg.BuildDefaults == nil {
		cfg.BuildDefaults = map[string]config.BuildDefaults{}
	}
	cfg.BuildDefaults[templatePath] = config.BuildDefaults{
		Env:       inputs.Env,
		Mode:      inputs.Mode,
		OutputDir: inputs.OutputDir,
		Params:    cloneParams(inputs.Parameters),
	}
	cfg.RecentTemplates = domaintpl.UpdateHistory(cfg.RecentTemplates, templatePath, templateHistoryLimit)
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
		return "", fmt.Errorf("stat template path: %w", err)
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
		if name == ".git" || name == ".esb" || name == "node_modules" || name == ".venv" {
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
