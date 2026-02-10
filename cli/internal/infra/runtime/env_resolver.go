// Where: cli/internal/infra/runtime/env_resolver.go
// What: Runtime environment inference service for deploy flows.
// Why: Keep Docker/gateway/staging probing out of command-layer code.
package runtime

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
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

// EnvInference captures an inferred environment name and its source.
type EnvInference struct {
	Env    string
	Source string
}

// EnvResolver infers deploy environment from runtime state.
type EnvResolver interface {
	InferEnvFromProject(composeProject, templatePath string) (EnvInference, error)
}

// DockerEnvResolver infers environment using Docker labels, gateway config mounts,
// and staged config directories.
type DockerEnvResolver struct{}

var errUnsupportedDockerClient = fmt.Errorf("unsupported docker client")

// NewEnvResolver constructs a runtime environment resolver.
func NewEnvResolver() EnvResolver {
	return DockerEnvResolver{}
}

// InferEnvFromProject tries multiple sources and returns the first stable env.
func (DockerEnvResolver) InferEnvFromProject(composeProject, templatePath string) (EnvInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return EnvInference{}, nil
	}

	var firstErr error
	inferred, err := inferEnvFromRunningContainerLabels(trimmed)
	if err != nil {
		firstErr = err
	}
	if inferred.Env != "" {
		return inferred, nil
	}

	inferred, err = inferEnvFromGateway(trimmed, templatePath)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	if inferred.Env != "" {
		return inferred, nil
	}

	inferred, err = inferEnvFromStaging(trimmed, templatePath)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	if inferred.Env != "" {
		return inferred, nil
	}
	return EnvInference{}, firstErr
}

func inferEnvFromRunningContainerLabels(composeProject string) (EnvInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return EnvInference{}, nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return EnvInference{}, fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, trimmed))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: false, Filters: filterArgs})
	if err != nil {
		return EnvInference{}, fmt.Errorf("list containers: %w", err)
	}
	return InferEnvFromContainerLabels(containers), nil
}

// InferEnvFromContainerLabels infers a unique env value from container labels.
func InferEnvFromContainerLabels(containers []container.Summary) EnvInference {
	envs := map[string]struct{}{}
	for _, ctr := range containers {
		if ctr.Labels == nil {
			continue
		}
		env := strings.TrimSpace(ctr.Labels[compose.ESBEnvLabel])
		if env == "" {
			continue
		}
		envs[env] = struct{}{}
		if len(envs) > 1 {
			return EnvInference{}
		}
	}
	if len(envs) != 1 {
		return EnvInference{}
	}
	for env := range envs {
		return EnvInference{Env: env, Source: "container label"}
	}
	return EnvInference{}
}

func inferEnvFromGateway(composeProject, templatePath string) (EnvInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return EnvInference{}, nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return EnvInference{}, fmt.Errorf("create docker client: %w", err)
	}
	rawClient, ok := client.(*dockerclient.Client)
	if !ok {
		return EnvInference{}, errUnsupportedDockerClient
	}

	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, trimmed))
	containers, err := rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return EnvInference{}, fmt.Errorf("list containers: %w", err)
	}
	if len(containers) == 0 {
		return EnvInference{}, nil
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
		return EnvInference{}, fmt.Errorf("inspect container: %w", err)
	}
	envMap := envSliceToMap(inspect.Config.Env)
	if env := strings.TrimSpace(envMap["ENV"]); env != "" {
		return EnvInference{Env: env, Source: "gateway env"}, nil
	}
	rootDir, err := staging.RootDir(templatePath)
	if err != nil {
		return EnvInference{}, nil
	}
	for _, mount := range inspect.Mounts {
		if mount.Destination != "/app/runtime-config" {
			continue
		}
		if strings.EqualFold(string(mount.Type), "bind") && mount.Source != "" {
			if env := InferEnvFromConfigPath(mount.Source, rootDir); env != "" {
				return EnvInference{Env: env, Source: "gateway config mount"}, nil
			}
		}
	}
	return EnvInference{}, nil
}

func inferEnvFromStaging(composeProject, templatePath string) (EnvInference, error) {
	trimmed := strings.TrimSpace(composeProject)
	if trimmed == "" {
		return EnvInference{}, nil
	}
	rootDir, err := staging.RootDir(templatePath)
	if err != nil {
		return EnvInference{}, err
	}
	envs, err := DiscoverStagingEnvs(rootDir, trimmed)
	if err != nil {
		return EnvInference{}, err
	}
	if len(envs) == 1 {
		return EnvInference{Env: envs[0], Source: "staging"}, nil
	}
	return EnvInference{}, nil
}

// InferEnvFromConfigPath infers env from a staging config directory path.
func InferEnvFromConfigPath(path, rootDir string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return ""
	}
	if filepath.Base(cleaned) != "config" {
		return ""
	}
	if strings.TrimSpace(rootDir) == "" {
		return ""
	}
	stagingRoot := filepath.Clean(rootDir) + string(filepath.Separator)
	if !strings.HasPrefix(cleaned+string(filepath.Separator), stagingRoot) {
		return ""
	}
	env := filepath.Base(filepath.Dir(cleaned))
	if env == "" || env == "." || env == string(filepath.Separator) {
		return ""
	}
	return env
}

// DiscoverStagingEnvs finds env directories under <staging>/<composeProject>/<env>/config.
func DiscoverStagingEnvs(rootDir, composeProject string) ([]string, error) {
	baseDir := filepath.Join(rootDir, composeProject)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read staging dir: %w", err)
	}
	envs := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(baseDir, entry.Name(), "config")
		if _, err := os.Stat(candidate); err == nil {
			envs[entry.Name()] = struct{}{}
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
