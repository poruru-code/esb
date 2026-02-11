// Where: cli/internal/command/deploy_runtime_env.go
// What: Runtime-aware environment resolution for deploy.
// Why: Align deploy env with running gateway and staged configs to avoid mismatches.
package command

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	runtimeinfra "github.com/poruru/edge-serverless-box/cli/internal/infra/runtime"
)

var errEnvMismatch = errors.New("environment mismatch")

func reconcileEnvWithRuntime(
	choice envChoice,
	composeProject string,
	templatePath string,
	isTTY bool,
	prompter interaction.Prompter,
	resolver runtimeinfra.EnvResolver,
	allowMismatch bool,
	errOut io.Writer,
) (envChoice, error) {
	if strings.TrimSpace(composeProject) == "" {
		return choice, nil
	}
	if resolver == nil {
		return choice, nil
	}

	inferred, err := resolver.InferEnvFromProject(composeProject, templatePath)
	if err != nil {
		writeWarningf(errOut, "Warning: failed to infer running env: %v\n", err)
	}
	if inferred.Env == "" || inferred.Env == choice.Value {
		return choice, nil
	}

	if allowMismatch && strings.TrimSpace(choice.Value) != "" {
		writeWarningf(
			errOut,
			"Warning: environment mismatch (running=%q, deploy=%q); keeping %q due to --force\n",
			inferred.Env,
			choice.Value,
			choice.Value,
		)
		return choice, nil
	}

	if strings.TrimSpace(choice.Value) == "" {
		choice.Value = inferred.Env
		choice.Source = inferred.Source
		choice.Explicit = false
		return choice, nil
	}

	if choice.Explicit {
		if isTTY && prompter != nil {
			selected, err := promptEnvMismatch(choice, inferred, prompter)
			if err != nil {
				return choice, err
			}
			return applyEnvSelection(choice, inferred, selected), nil
		}
		return envChoice{}, fmt.Errorf(
			"%w: running env uses %q (%s), deploy uses %q (use --force to override)",
			errEnvMismatch,
			inferred.Env,
			inferred.Source,
			choice.Value,
		)
	}

	if isTTY && prompter != nil {
		selected, err := promptEnvMismatch(choice, inferred, prompter)
		if err != nil {
			return choice, err
		}
		return applyEnvSelection(choice, inferred, selected), nil
	}

	choice.Value = inferred.Env
	choice.Source = inferred.Source
	choice.Explicit = false
	return choice, nil
}

func promptEnvMismatch(
	current envChoice,
	inferred runtimeinfra.EnvInference,
	prompter interaction.Prompter,
) (string, error) {
	title := fmt.Sprintf(
		"Environment mismatch (running: %s, current: %s)",
		inferred.Env,
		current.Value,
	)
	options := []interaction.SelectOption{
		{
			Label: fmt.Sprintf("Use running env %q (recommended)", inferred.Env),
			Value: inferred.Env,
		},
		{
			Label: fmt.Sprintf("Keep current env %q", current.Value),
			Value: current.Value,
		},
	}
	selected, err := prompter.SelectValue(title, options)
	if err != nil {
		return "", fmt.Errorf("prompt env mismatch: %w", err)
	}
	return selected, nil
}

func applyEnvSelection(
	current envChoice,
	inferred runtimeinfra.EnvInference,
	selected string,
) envChoice {
	if selected == inferred.Env {
		return envChoice{
			Value:    inferred.Env,
			Source:   inferred.Source,
			Explicit: true,
		}
	}
	current.Explicit = true
	if current.Source == "default" {
		current.Source = "prompt"
	}
	return current
}
