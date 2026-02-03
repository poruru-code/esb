// Where: cli/internal/commands/deploy_runtime_env.go
// What: Runtime-aware environment resolution for deploy.
// Why: Align deploy env with running gateway and staged configs to avoid mismatches.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

type envInference struct {
	Env    string
	Source string
}

func reconcileEnvWithRuntime(
	choice envChoice,
	composeProject string,
	isTTY bool,
	prompter interaction.Prompter,
	allowMismatch bool,
) (envChoice, error) {
	if strings.TrimSpace(composeProject) == "" {
		return choice, nil
	}

	inferred, err := inferEnvFromGateway(composeProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to inspect running gateway env: %v\n", err)
	}
	if inferred.Env == "" {
		fallback, err := inferEnvFromStaging(composeProject)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to inspect staging env: %v\n", err)
		} else if fallback.Env != "" {
			inferred = fallback
		}
	}
	if inferred.Env == "" || inferred.Env == choice.Value {
		return choice, nil
	}

	if allowMismatch && strings.TrimSpace(choice.Value) != "" {
		fmt.Fprintf(
			os.Stderr,
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
			"environment mismatch: running gateway uses %q (%s), deploy uses %q (use --force to override)",
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

func promptEnvMismatch(current envChoice, inferred envInference, prompter interaction.Prompter) (string, error) {
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
	return prompter.SelectValue(title, options)
}

func applyEnvSelection(current envChoice, inferred envInference, selected string) envChoice {
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

func inferEnvFromGateway(composeProject string) (envInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return envInference{}, nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return envInference{}, err
	}
	rawClient, ok := client.(*dockerclient.Client)
	if !ok {
		return envInference{}, fmt.Errorf("unsupported docker client")
	}

	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, trimmed))
	containers, err := rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return envInference{}, err
	}
	if len(containers) == 0 {
		return envInference{}, nil
	}
	selected := containers[0]
	for _, ctr := range containers {
		if strings.EqualFold(ctr.State, "running") {
			selected = ctr
			break
		}
	}
	inspect, err := rawClient.ContainerInspect(ctx, selected.ID)
	if err != nil {
		return envInference{}, err
	}
	envMap := envSliceToMap(inspect.Config.Env)
	if env := strings.TrimSpace(envMap["ENV"]); env != "" {
		return envInference{Env: env, Source: "gateway env"}, nil
	}
	for _, mount := range inspect.Mounts {
		if mount.Destination != "/app/runtime-config" {
			continue
		}
		if strings.EqualFold(string(mount.Type), "bind") && mount.Source != "" {
			if env := inferEnvFromConfigPath(mount.Source); env != "" {
				return envInference{Env: env, Source: "gateway config mount"}, nil
			}
		}
	}
	return envInference{}, nil
}

func inferEnvFromConfigPath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return ""
	}
	if filepath.Base(cleaned) != "config" {
		return ""
	}
	stagingRoot := filepath.Clean(staging.RootDir()) + string(filepath.Separator)
	if !strings.HasPrefix(cleaned+string(filepath.Separator), stagingRoot) {
		return ""
	}
	env := filepath.Base(filepath.Dir(cleaned))
	if env == "" || env == "." || env == string(filepath.Separator) {
		return ""
	}
	return env
}

func inferEnvFromStaging(composeProject string) (envInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return envInference{}, nil
	}
	envs, err := discoverStagingEnvs(staging.RootDir(), trimmed)
	if err != nil {
		return envInference{}, err
	}
	if len(envs) == 1 {
		return envInference{Env: envs[0], Source: "staging"}, nil
	}
	return envInference{}, nil
}

func discoverStagingEnvs(rootDir, composeProject string) ([]string, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	needle := composeProject + "-"
	envs := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, needle) {
			continue
		}
		baseDir := filepath.Join(rootDir, name)
		subdirs, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}
		for _, sub := range subdirs {
			if !sub.IsDir() {
				continue
			}
			candidate := filepath.Join(baseDir, sub.Name(), "config")
			if _, err := os.Stat(candidate); err == nil {
				envs[sub.Name()] = struct{}{}
			}
		}
	}

	if len(envs) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(envs))
	for env := range envs {
		out = append(out, env)
	}
	sort.Strings(out)
	return out, nil
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		out[key] = value
	}
	return out
}
