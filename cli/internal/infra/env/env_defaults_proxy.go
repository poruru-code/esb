// Where: cli/internal/infra/env/env_defaults_proxy.go
// What: Proxy and registry host environment normalization for runtime commands.
// Why: Keep network proxy compatibility rules isolated from other env defaults.
package env

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
)

// applyProxyDefaults ensures that proxy-related environment variables are consistent
// and that NO_PROXY includes necessary local targets to avoid connection issues
// in proxy environments. Matches the behavior of the Python E2E runner.
func applyProxyDefaults() error {
	proxyKeys := []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"}
	hasProxy := false
	for _, key := range proxyKeys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			hasProxy = true
			break
		}
	}

	existingNoProxy := os.Getenv("NO_PROXY")
	if existingNoProxy == "" {
		existingNoProxy = os.Getenv("no_proxy")
	}

	extraNoProxy, err := envutil.GetHostEnv(constants.HostSuffixNoProxyExtra)
	if err != nil {
		return fmt.Errorf("get host env %s: %w", constants.HostSuffixNoProxyExtra, err)
	}

	if !hasProxy && existingNoProxy == "" && extraNoProxy == "" {
		return nil
	}

	defaultTargets := []string{
		"agent",
		"database",
		"gateway",
		"local-proxy",
		"localhost",
		"registry",
		"runtime-node",
		"s3-storage",
		"victorialogs",
		"::1",
		"10.88.0.0/16",
		"10.99.0.1",
		"127.0.0.1",
		"172.20.0.0/16",
	}

	split := func(val string) []string {
		if val == "" {
			return nil
		}
		val = strings.ReplaceAll(val, ";", ",")
		parts := strings.Split(val, ",")
		var cleaned []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		return cleaned
	}

	var merged []string
	seen := make(map[string]bool)

	addItem := func(item string) {
		if item != "" && !seen[item] {
			merged = append(merged, item)
			seen[item] = true
		}
	}

	for _, item := range split(existingNoProxy) {
		addItem(item)
	}
	for _, item := range defaultTargets {
		addItem(item)
	}
	for _, item := range split(extraNoProxy) {
		addItem(item)
	}

	if len(merged) > 0 {
		val := strings.Join(merged, ",")
		_ = os.Setenv("NO_PROXY", val)
		_ = os.Setenv("no_proxy", val)
	}

	// Sync upper/lower case versions for subprocesses.
	sync := func(upper, lower string) {
		u := os.Getenv(upper)
		l := os.Getenv(lower)
		if u != "" && l == "" {
			_ = os.Setenv(lower, u)
		} else if l != "" && u == "" {
			_ = os.Setenv(upper, l)
		}
	}
	sync("HTTP_PROXY", "http_proxy")
	sync("HTTPS_PROXY", "https_proxy")
	return nil
}

func normalizeRegistryEnv() error {
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		return fmt.Errorf("resolve host env key for registry: %w", err)
	}
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	if !strings.HasSuffix(value, "/") {
		value += "/"
		_ = os.Setenv(key, value)
	}
	return nil
}
