// Where: cli/internal/infra/build/go_builder_registry.go
// What: Registry host resolution and readiness probes for build pipeline.
// Why: Keep GoBuilder orchestration focused by isolating registry wait details.
package build

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
)

func isLocalRegistryHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "registry", "localhost", "127.0.0.1":
		return true
	default:
		return false
	}
}

func resolveRegistryHost(registry string) string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	if slash := strings.Index(trimmed, "/"); slash != -1 {
		trimmed = trimmed[:slash]
	}
	host := strings.TrimSpace(trimmed)
	if host == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		return strings.TrimSpace(splitHost)
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	if colon := strings.Index(host, ":"); colon != -1 {
		host = host[:colon]
	}
	return host
}

func resolveHostRegistryAddress() (string, bool) {
	if value := strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")); value != "" {
		return strings.TrimPrefix(value, "http://"), true
	}
	port := strings.TrimSpace(os.Getenv(constants.EnvPortRegistry))
	if port == "" {
		port = "5010"
	}
	return fmt.Sprintf("127.0.0.1:%s", port), false
}

func waitForRegistry(registry string, timeout time.Duration) error {
	if strings.TrimSpace(os.Getenv("ESB_REGISTRY_WAIT")) == "0" {
		return nil
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(registry), "/")
	if trimmed == "" {
		return nil
	}
	url := fmt.Sprintf("http://%s/v2/", trimmed)
	client := registryWaitHTTPClient(trimmed)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create registry request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("registry not responding at %s", url)
}

func registryWaitHTTPClient(registry string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if shouldBypassRegistryProxy(registry) {
		transport.Proxy = nil
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
	}
}

func shouldBypassRegistryProxy(registry string) bool {
	host := resolveRegistryHost(registry)
	normalized := strings.ToLower(strings.TrimSpace(host))
	if normalized == "" {
		return false
	}
	if isLocalRegistryHost(normalized) || normalized == "host.docker.internal" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}
