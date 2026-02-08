// Where: cli/internal/infra/build/bake_proxy.go
// What: Buildx proxy environment resolution and builder proxy checks.
// Why: Keep proxy-specific logic separate from generic bake orchestration.
package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

var buildxProxyEnvKeys = []string{
	"HTTP_PROXY",
	"http_proxy",
	"HTTPS_PROXY",
	"https_proxy",
	"NO_PROXY",
	"no_proxy",
}

var buildxProxyEnvKeySet = map[string]struct{}{
	"HTTP_PROXY":  {},
	"http_proxy":  {},
	"HTTPS_PROXY": {},
	"https_proxy": {},
	"NO_PROXY":    {},
	"no_proxy":    {},
}

func resolveProxyValue(upper, lower, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(upper)); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(lower)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func readDockerConfigProxy() map[string]string {
	configDir := strings.TrimSpace(os.Getenv("DOCKER_CONFIG"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		configDir = filepath.Join(home, ".docker")
	}
	configPath := filepath.Join(configDir, "config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var parsed struct {
		Proxies map[string]struct {
			HTTPProxy  string `json:"httpProxy"`
			HTTPSProxy string `json:"httpsProxy"`
			NoProxy    string `json:"noProxy"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	defaults, ok := parsed.Proxies["default"]
	if !ok {
		return nil
	}
	values := make(map[string]string)
	if value := strings.TrimSpace(defaults.HTTPProxy); value != "" {
		values["httpProxy"] = value
	}
	if value := strings.TrimSpace(defaults.HTTPSProxy); value != "" {
		values["httpsProxy"] = value
	}
	if value := strings.TrimSpace(defaults.NoProxy); value != "" {
		values["noProxy"] = value
	}
	return values
}

func buildxProxyEnvMap() map[string]string {
	defaults := readDockerConfigProxy()
	pairs := []struct {
		upper      string
		lower      string
		configName string
	}{
		{upper: "HTTP_PROXY", lower: "http_proxy", configName: "httpProxy"},
		{upper: "HTTPS_PROXY", lower: "https_proxy", configName: "httpsProxy"},
		{upper: "NO_PROXY", lower: "no_proxy", configName: "noProxy"},
	}
	envs := make(map[string]string)
	for _, pair := range pairs {
		value := resolveProxyValue(pair.upper, pair.lower, defaults[pair.configName])
		if value == "" {
			continue
		}
		envs[pair.upper] = value
		envs[pair.lower] = value
	}
	return envs
}

func buildxProxyDriverEnvMap() map[string]string {
	envs := buildxProxyEnvMap()
	driverEnv := make(map[string]string)
	for key, value := range envs {
		if strings.Contains(value, ",") && strings.EqualFold(key, "no_proxy") {
			continue
		}
		driverEnv[key] = value
	}
	return driverEnv
}

func buildxProxyDriverOptsFromMap(envs map[string]string) []string {
	if len(envs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envs))
	for key := range envs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	opts := make([]string, 0, len(keys))
	for _, key := range keys {
		opts = append(opts, fmt.Sprintf("env.%s=%s", key, envs[key]))
	}
	return opts
}

func buildxBuilderProxyEnv(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	builder string,
) (map[string]string, error) {
	containerName := fmt.Sprintf("buildx_buildkit_%s0", builder)
	output, err := runner.RunOutput(
		ctx,
		repoRoot,
		"docker",
		"inspect",
		"-f",
		"{{range .Config.Env}}{{println .}}{{end}}",
		containerName,
	)
	if err != nil {
		return nil, err
	}
	env := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if _, ok := buildxProxyEnvKeySet[key]; ok {
			env[key] = value
		}
	}
	return env, nil
}

func buildxBuilderProxyMismatch(
	ctx context.Context,
	runner compose.CommandRunner,
	repoRoot string,
	builder string,
	desired map[string]string,
) (bool, error) {
	existing, err := buildxBuilderProxyEnv(ctx, runner, repoRoot, builder)
	if err != nil {
		return false, err
	}
	for _, key := range buildxProxyEnvKeys {
		desiredValue := strings.TrimSpace(desired[key])
		existingValue := strings.TrimSpace(existing[key])
		if desiredValue == "" {
			if existingValue != "" {
				return true, nil
			}
			continue
		}
		if existingValue != desiredValue {
			return true, nil
		}
	}
	return false, nil
}
