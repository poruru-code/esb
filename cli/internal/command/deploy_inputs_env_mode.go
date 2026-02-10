// Where: cli/internal/command/deploy_inputs_env_mode.go
// What: Environment and runtime mode resolution helpers for deploy inputs.
// Why: Keep prompt-driven env/mode logic separate from the main resolve flow.
package command

import (
	"fmt"
	"os"
	"strings"

	runtimecfg "github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
)

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
	resolver runtimeinfra.EnvResolver,
	previous string,
) (envChoice, error) {
	if trimmed := strings.TrimSpace(envValue); trimmed != "" {
		return envChoice{Value: trimmed, Source: "flag", Explicit: true}, nil
	}
	if env := strings.TrimSpace(stack.Env); env != "" {
		return envChoice{Value: env, Source: "stack", Explicit: false}, nil
	}
	if project := strings.TrimSpace(composeProject); project != "" && resolver != nil {
		if inferred, err := resolver.InferEnvFromProject(project, ""); err == nil && strings.TrimSpace(inferred.Env) != "" {
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
