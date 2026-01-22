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
	envApplier := helpers.NewRuntimeEnvApplier(repoResolver)
	ui := ports.NewLegacyUI(out)
	return &buildCommand{
		builder:    deps.Builder,
		envApplier: envApplier,
		ui:         ui,
	}, nil
}

func (c *buildCommand) Run(inputs buildInputs, flags BuildCmd) error {
	request := workflows.BuildRequest{
		Context:      inputs.Context,
		Env:          inputs.Env,
		Mode:         inputs.Mode,
		TemplatePath: inputs.TemplatePath,
		OutputDir:    inputs.OutputDir,
		Parameters:   inputs.Parameters,
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

	templatePath, err := resolveBuildTemplate(cli.Template, isTTY, prompter)
	if err != nil {
		return buildInputs{}, err
	}

	env, err := resolveBuildEnv(cli.EnvFlag, isTTY, prompter)
	if err != nil {
		return buildInputs{}, err
	}

	mode, err := resolveBuildMode(cli.Build.Mode, isTTY, prompter)
	if err != nil {
		return buildInputs{}, err
	}

	outputDir := strings.TrimSpace(cli.Build.Output)

	params, err := promptTemplateParameters(templatePath, isTTY, prompter)
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

	return buildInputs{
		Context:      ctx,
		Env:          env,
		Mode:         mode,
		TemplatePath: templatePath,
		OutputDir:    outputDir,
		Parameters:   params,
	}, nil
}

func resolveBuildTemplate(value string, isTTY bool, prompter interaction.Prompter) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return normalizeTemplatePath(trimmed)
	}
	if !isTTY || prompter == nil {
		return "", fmt.Errorf("template path is required")
	}
	candidates := discoverTemplateCandidates()
	title := "Template path"
	if len(candidates) > 0 {
		title = fmt.Sprintf("Template path (default: %s)", candidates[0])
	}
	input, err := prompter.Input(title, candidates)
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		if len(candidates) > 0 {
			if path, err := normalizeTemplatePath(candidates[0]); err == nil {
				return path, nil
			}
		}
		if path, err := normalizeTemplatePath("."); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("template path is required")
	}
	return normalizeTemplatePath(input)
}

func discoverTemplateCandidates() []string {
	candidates := []string{}
	for _, name := range []string{"template.yaml", "template.yml"} {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func resolveBuildEnv(value string, isTTY bool, prompter interaction.Prompter) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, nil
	}
	if !isTTY || prompter == nil {
		return "", fmt.Errorf("environment is required")
	}
	input, err := prompter.Input("Environment name", nil)
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("environment is required")
	}
	return input, nil
}

func resolveBuildMode(value string, isTTY bool, prompter interaction.Prompter) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed != "" {
		return normalizeMode(trimmed)
	}
	if !isTTY || prompter == nil {
		return "", fmt.Errorf("mode is required")
	}
	selected, err := prompter.Select("Runtime mode", []string{"docker", "containerd", "firecracker"})
	if err != nil {
		return "", err
	}
	selected = strings.TrimSpace(strings.ToLower(selected))
	if selected == "" {
		return "", fmt.Errorf("mode is required")
	}
	return normalizeMode(selected)
}

func normalizeMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "docker", "containerd", "firecracker":
		return strings.ToLower(strings.TrimSpace(mode)), nil
	default:
		return "", fmt.Errorf("invalid mode %q (expected docker, containerd, or firecracker)", mode)
	}
}

func normalizeTemplatePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
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

func promptTemplateParameters(templatePath string, isTTY bool, prompter interaction.Prompter) (map[string]string, error) {
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

		if !isTTY || prompter == nil {
			if hasDefault {
				values[name] = defaultStr
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
		} else {
			title = fmt.Sprintf("%s [Required]", label)
		}

		input, err := prompter.Input(title, nil)
		if err != nil {
			return nil, err
		}
		input = strings.TrimSpace(input)
		if input == "" && hasDefault {
			input = defaultStr
		}
		if input == "" && !hasDefault {
			return nil, fmt.Errorf("parameter %q is required", name)
		}
		values[name] = input
	}

	return values, nil
}
