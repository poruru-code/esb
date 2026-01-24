// Where: cli/internal/commands/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/helpers"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
	"github.com/poruru/edge-serverless-box/meta"
)

// Builder defines the interface for building Lambda function images.
// Implementations generate Dockerfiles and build container images.
type Builder = ports.Builder

// runBuild executes the 'build' command which generates Dockerfiles
// and builds container images for all Lambda functions in the SAM template.
func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	cmd, err := newBuildCommand(deps.Build, repoResolver, out)
	if err != nil {
		return exitWithError(out, err)
	}

	inputs, err := resolveBuildInputs(cli, deps)
	if err != nil {
		return exitWithError(out, err)
	}

	if err := cmd.Run(inputs, cli.Build); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type buildCommand struct {
	builder    ports.Builder
	envApplier ports.RuntimeEnvApplier
	ui         ports.UserInterface
}

func newBuildCommand(deps BuildDeps, repoResolver func(string) (string, error), out io.Writer) (*buildCommand, error) {
	if deps.Builder == nil {
		return nil, fmt.Errorf("build: builder not configured")
	}
	envApplier, err := helpers.NewRuntimeEnvApplier(repoResolver)
	if err != nil {
		return nil, err
	}
	ui := ports.NewLegacyUI(out)
	return &buildCommand{
		builder:    deps.Builder,
		envApplier: envApplier,
		ui:         ui,
	}, nil
}

func (c *buildCommand) Run(inputs buildInputs, flags BuildCmd) error {
	version, err := resolveBrandVersion()
	if err != nil {
		return err
	}
	tag, err := resolveBrandTag(version)
	if err != nil {
		return err
	}
	request := workflows.BuildRequest{
		Context:      inputs.Context,
		Env:          inputs.Env,
		Mode:         inputs.Mode,
		TemplatePath: inputs.TemplatePath,
		OutputDir:    inputs.OutputDir,
		Parameters:   inputs.Parameters,
		Version:      version,
		Tag:          tag,
		NoCache:      flags.NoCache,
		Verbose:      flags.Verbose,
	}
	return workflows.NewBuildWorkflow(c.builder, c.envApplier, c.ui).Run(request)
}

type buildInputs struct {
	Context      state.Context
	Env          string
	Mode         string
	TemplatePath string
	OutputDir    string
	Parameters   map[string]string
}

func resolveBuildInputs(cli CLI, deps Dependencies) (buildInputs, error) {
	isTTY := interaction.IsTerminal(os.Stdin)
	prompter := deps.Prompter

	var last buildInputs
	for {
		templatePath, err := resolveBuildTemplate(cli.Template, isTTY, prompter, last.TemplatePath)
		if err != nil {
			return buildInputs{}, err
		}
		env, err := resolveBuildEnv(cli.EnvFlag, isTTY, prompter, last.Env)
		if err != nil {
			return buildInputs{}, err
		}

		mode, err := resolveBuildMode(cli.Build.Mode, isTTY, prompter, last.Mode)
		if err != nil {
			return buildInputs{}, err
		}

		outputDir, err := resolveBuildOutput(
			cli.Build.Output,
			templatePath,
			env,
			isTTY,
			prompter,
			last.OutputDir,
		)
		if err != nil {
			return buildInputs{}, err
		}

		params, err := promptTemplateParameters(templatePath, isTTY, prompter, last.Parameters)
		if err != nil {
			return buildInputs{}, err
		}

		projectDir, err := os.Getwd()
		if err != nil {
			return buildInputs{}, err
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

		inputs := buildInputs{
			Context:      ctx,
			Env:          env,
			Mode:         mode,
			TemplatePath: templatePath,
			OutputDir:    outputDir,
			Parameters:   params,
		}

		confirmed, err := confirmBuildInputs(inputs, isTTY, prompter)
		if err != nil {
			return buildInputs{}, err
		}
		if confirmed {
			return inputs, nil
		}
		last = inputs
	}
}

func resolveBuildTemplate(
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

func resolveBrandVersion() (string, error) {
	key, err := envutil.HostEnvKey(constants.HostSuffixVersion)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(os.Getenv(key))
	if version == "" {
		return "", fmt.Errorf("ERROR: %s is required", key)
	}
	return version, nil
}

func resolveBrandTag(version string) (string, error) {
	tagKey, err := envutil.HostEnvKey(constants.HostSuffixTag)
	if err != nil {
		return "", err
	}
	versionKey, err := envutil.HostEnvKey(constants.HostSuffixVersion)
	if err != nil {
		return "", err
	}
	tag := strings.TrimSpace(os.Getenv(tagKey))
	if tag == "" {
		tag = version
		_ = os.Setenv(tagKey, tag)
	}
	if tag == version {
		return tag, nil
	}
	if tag == "latest" && strings.HasPrefix(version, "0.0.0-dev.") {
		return tag, nil
	}
	return "", fmt.Errorf("ERROR: %s must match %s", tagKey, versionKey)
}

func discoverTemplateCandidates() []string {
	candidates := []string{}
	baseDir := resolvePromptBaseDir()
	for _, name := range []string{"template.yaml", "template.yml"} {
		if info, err := os.Stat(filepath.Join(baseDir, name)); err == nil && !info.IsDir() {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func resolveBuildEnv(
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

func resolveBuildMode(
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

func resolveBuildOutput(
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

	suggestions := []string{}
	if prev := strings.TrimSpace(previous); prev != "" {
		suggestions = append(suggestions, prev)
	}
	if defaultBase != "" && defaultBase != previous {
		suggestions = append(suggestions, defaultBase)
	}

	title := fmt.Sprintf("Output directory (default: %s)", defaultResolved)
	input, err := prompter.Input(title, suggestions)
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	cleaned := filepath.Clean(input)
	if filepath.Clean(defaultResolved) == cleaned {
		return defaultBase, nil
	}
	return cleaned, nil
}

func normalizeMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "docker", "containerd":
		return strings.ToLower(strings.TrimSpace(mode)), nil
	default:
		return "", fmt.Errorf("invalid mode %q (expected docker or containerd)", mode)
	}
}

func normalizeTemplatePath(path string) (string, error) {
	baseDir := resolvePromptBaseDir()
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(baseDir, absPath)
	}
	absPath = filepath.Clean(absPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return absPath, nil
	}
	for _, name := range []string{"template.yaml", "template.yml"} {
		candidate := filepath.Join(absPath, name)
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no template.yaml or template.yml found in directory: %s", path)
}

func resolvePromptBaseDir() string {
	if pwd := strings.TrimSpace(os.Getenv("PWD")); pwd != "" {
		return pwd
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func promptTemplateParameters(
	templatePath string,
	isTTY bool,
	prompter interaction.Prompter,
	previous map[string]string,
) (map[string]string, error) {
	params, err := parseTemplateParameters(templatePath)
	if err != nil || len(params) == 0 {
		return map[string]string{}, err
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
				// Allow empty string if type is explicitely "String"
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

func confirmBuildInputs(inputs buildInputs, isTTY bool, prompter interaction.Prompter) (bool, error) {
	if !isTTY || prompter == nil {
		return true, nil
	}

	output := resolveOutputSummary(inputs.TemplatePath, inputs.OutputDir, inputs.Env)

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

func resolveOutputSummary(templatePath, outputDir, env string) string {
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
