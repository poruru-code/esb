// Where: cli/internal/commands/deploy.go
// What: Deploy command implementation.
// Why: Orchestrate deploy operations in a testable way.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/sam"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
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
	builder       ports.Builder
	envApplier    ports.RuntimeEnvApplier
	ui            ports.UserInterface
	composeRunner compose.CommandRunner
}

func newDeployCommand(
	deployDeps DeployDeps,
	repoResolver func(string) (string, error),
	out io.Writer,
) (*deployCommand, error) {
	if deployDeps.Builder == nil {
		return nil, fmt.Errorf("deploy: builder not configured")
	}
	envApplier, err := helpers.NewRuntimeEnvApplier(repoResolver)
	if err != nil {
		return nil, err
	}
	ui := ports.NewLegacyUI(out)
	return &deployCommand{
		builder:       deployDeps.Builder,
		envApplier:    envApplier,
		ui:            ui,
		composeRunner: compose.ExecRunner{},
	}, nil
}

func (c *deployCommand) Run(inputs deployInputs, flags DeployCmd) error {
	tag, err := resolveBrandTag()
	if err != nil {
		return err
	}
	request := workflows.DeployRequest{
		Context:      inputs.Context,
		Env:          inputs.Env,
		Mode:         inputs.Mode,
		TemplatePath: inputs.TemplatePath,
		OutputDir:    inputs.OutputDir,
		Parameters:   inputs.Parameters,
		Tag:          tag,
		NoCache:      flags.NoCache,
		Verbose:      flags.Verbose,
	}
	return workflows.NewDeployWorkflow(c.builder, c.envApplier, c.ui, c.composeRunner).Run(request)
}

type deployInputs struct {
	Context      state.Context
	Env          string
	Mode         string
	TemplatePath string
	OutputDir    string
	Parameters   map[string]string
}

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
		env, err := resolveDeployEnv(cli.EnvFlag, isTTY, prompter, prevEnv)
		if err != nil {
			return deployInputs{}, err
		}

		prevMode := strings.TrimSpace(last.Mode)
		if prevMode == "" {
			prevMode = strings.TrimSpace(stored.Mode)
		}
		mode, err := resolveDeployMode(cli.Deploy.Mode, isTTY, prompter, prevMode)
		if err != nil {
			return deployInputs{}, err
		}

		prevOutput := strings.TrimSpace(last.OutputDir)
		if prevOutput == "" {
			prevOutput = strings.TrimSpace(stored.OutputDir)
		}
		outputDir, err := resolveDeployOutput(
			cli.Deploy.Output,
			templatePath,
			env,
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
			return deployInputs{}, err
		}

		composeProject := strings.TrimSpace(os.Getenv(constants.EnvProjectName))
		if composeProject == "" {
			brandName := strings.ToLower(strings.TrimSpace(os.Getenv("CLI_CMD")))
			if brandName == "" {
				brandName = meta.Slug
			}
			composeProject = fmt.Sprintf("%s-%s", brandName, strings.ToLower(env))
		}

		ctx := state.Context{
			ProjectDir:     projectDir,
			TemplatePath:   templatePath,
			OutputDir:      outputDir,
			Env:            env,
			Mode:           mode,
			ComposeProject: composeProject,
		}

		inputs := deployInputs{
			Context:      ctx,
			Env:          env,
			Mode:         mode,
			TemplatePath: templatePath,
			OutputDir:    outputDir,
			Parameters:   params,
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
		return "", fmt.Errorf("template path is required")
	}
	for {
		candidates := discoverTemplateCandidates()
		suggestions := make([]string, 0, len(candidates)+1)
		if strings.TrimSpace(previous) != "" {
			suggestions = append(suggestions, previous)
		}
		for _, candidate := range candidates {
			if candidate == previous {
				continue
			}
			suggestions = append(suggestions, candidate)
		}
		title := "Template path"
		if strings.TrimSpace(previous) != "" {
			title = fmt.Sprintf("Template path (default: %s)", previous)
		} else if len(candidates) > 0 {
			title = fmt.Sprintf("Template path (default: %s)", candidates[0])
		}
		input, err := prompter.Input(title, suggestions)
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			if strings.TrimSpace(previous) != "" {
				if path, err := normalizeTemplatePath(previous); err == nil {
					return path, nil
				}
			}
			if len(candidates) > 0 {
				if path, err := normalizeTemplatePath(candidates[0]); err == nil {
					return path, nil
				}
			}
			if path, err := normalizeTemplatePath("."); err == nil {
				return path, nil
			}
			fmt.Fprintln(os.Stderr, "Template path is required.")
			continue
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
) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, nil
	}
	if !isTTY || prompter == nil {
		return "", fmt.Errorf("environment is required")
	}
	defaultValue := strings.TrimSpace(previous)
	if defaultValue == "" {
		defaultValue = "default"
	}
	title := fmt.Sprintf("Environment name (default: %s)", defaultValue)
	input, err := prompter.Input(title, []string{defaultValue})
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

func resolveDeployMode(
	value string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed != "" {
		return normalizeMode(trimmed)
	}
	if !isTTY || prompter == nil {
		return "", fmt.Errorf("mode is required")
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
			return "", err
		}
		selected = strings.TrimSpace(strings.ToLower(selected))
		if selected == "" {
			fmt.Fprintln(os.Stderr, "Runtime mode is required.")
			continue
		}
		return normalizeMode(selected)
	}
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
		return "", err
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

	output := resolveDeployOutputSummary(inputs.TemplatePath, inputs.OutputDir, inputs.Env)

	summaryLines := []string{
		fmt.Sprintf("Template: %s", inputs.TemplatePath),
		fmt.Sprintf("Env: %s", inputs.Env),
		fmt.Sprintf("Mode: %s", inputs.Mode),
		fmt.Sprintf("Output: %s", output),
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
		return false, err
	}
	return choice == "proceed", nil
}

func resolveDeployOutputSummary(templatePath, outputDir, env string) string {
	baseDir := filepath.Dir(templatePath)
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return filepath.Join(baseDir, meta.OutputDir, env)
	}
	path := trimmed
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Join(filepath.Clean(path), env)
}

// storedDeployDefaults mirrors storedBuildDefaults for deploy.
type storedDeployDefaults struct {
	Env       string
	Mode      string
	OutputDir string
	Params    map[string]string
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
		return err
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
	return config.SaveGlobalConfig(cfgPath, cfg)
}

// Helper functions from deleted build.go

func resolveBrandTag() (string, error) {
	// Use brand-prefixed environment variable (e.g., ESB_TAG)
	key, err := envutil.HostEnvKey(constants.HostSuffixTag)
	if err != nil {
		return "latest", nil
	}
	tag := os.Getenv(key)
	if tag != "" {
		return tag, nil
	}
	return "latest", nil
}

func normalizeTemplatePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("template path is empty")
	}

	// Expand ~ to home directory
	expanded, err := expandHomePath(path)
	if err != nil {
		return "", err
	}

	cleaned := filepath.Clean(expanded)
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", err
	}

	// If it's a file, return its absolute path
	if !info.IsDir() {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	// If it's a directory, look for template.yaml or template.yml
	for _, name := range []string{"template.yaml", "template.yml"} {
		candidate := filepath.Join(cleaned, name)
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}

	// No template file found in directory
	return "", fmt.Errorf("no template.yaml or template.yml found in directory: %s", cleaned)
}

// expandHomePath expands ~ to the user's home directory
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
		candidates = append(candidates, name)
	}
	return candidates
}

func normalizeMode(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "docker", "containerd":
		return m, nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

func promptTemplateParameters(templatePath string, isTTY bool, prompter interaction.Prompter, previous map[string]string) (map[string]string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return map[string]string{}, err
	}

	data, err := sam.DecodeYAML(string(content))
	if err != nil {
		return map[string]string{}, err
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
			return nil, fmt.Errorf("parameter %q requires a value", name)
		}

		label := name
		if param.Description != "" {
			label = fmt.Sprintf("%s (%s)", name, param.Description)
		}

		var title string
		if hasDefault {
			displayDefault := defaultStr
			if displayDefault == "" {
				displayDefault = "''"
			}
			title = fmt.Sprintf("%s [Default: %s]", label, displayDefault)
		} else if prevValue != "" {
			title = fmt.Sprintf("%s [Previous: %s]", label, prevValue)
		} else {
			title = fmt.Sprintf("%s [Required]", label)
		}

		suggestions := []string{}
		if prevValue != "" {
			suggestions = append(suggestions, prevValue)
		}
		for {
			input, err := prompter.Input(title, suggestions)
			if err != nil {
				return nil, err
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

// samParameter represents a SAM template parameter definition
type samParameter struct {
	Type        string
	Description string
	Default     any
}

// extractSAMParameters extracts parameter definitions from SAM template data
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

// asMap converts an interface to a map[string]any
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
