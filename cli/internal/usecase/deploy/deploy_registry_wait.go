// Where: cli/internal/usecase/deploy/deploy_registry_wait.go
// What: Registry readiness checks and probe URL resolution for deploy.
// Why: Keep host-side registry wait behavior separate from workflow orchestration.
package deploy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// RegistryWaiter checks registry readiness.
type RegistryWaiter func(registry string, timeout time.Duration) error

func defaultRegistryWaiter(registry string, timeout time.Duration) error {
	if strings.TrimSpace(registry) == "" {
		return nil
	}
	probeURLs := resolveRegistryProbeURLs(registry)
	if len(probeURLs) == 0 {
		return fmt.Errorf("invalid registry address: %s", registry)
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		for _, probeURL := range probeURLs {
			client := registryWaitHTTPClient(probeURL)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, probeURL, nil)
			if err != nil {
				return fmt.Errorf("create registry request: %w", err)
			}
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				// 2xx-4xx means endpoint is reachable and the registry is up.
				if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
					return nil
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%w at %s", errRegistryNotResponding, probeURLs[0])
}

func (w Workflow) waitRegistryAndServices(req Request) error {
	if req.BuildOnly {
		return nil
	}

	registry := w.resolveRegistryAddress()
	if w.RegistryWaiter != nil {
		if err := w.RegistryWaiter(registry, 60*time.Second); err != nil {
			return fmt.Errorf("registry not ready: %w", err)
		}
	}

	// Check gateway/agent status (warning only).
	w.checkServicesStatus(req.Context.ComposeProject, req.Mode)
	return nil
}

func resolveRegistryProbeURLs(registry string) []string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && strings.TrimSpace(parsed.Host) != "" {
			parsed.Path = "/v2/"
			parsed.RawPath = ""
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return []string{parsed.String()}
		}
	}

	host := strings.TrimPrefix(trimmed, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimSuffix(host, "/")
	if slash := strings.Index(host, "/"); slash != -1 {
		host = host[:slash]
	}
	if host == "" {
		return nil
	}

	schemes := []string{"https", "http"}
	if shouldBypassRegistryProxy(resolveRegistryHost(host)) {
		schemes = []string{"http", "https"}
	}

	urls := make([]string, 0, len(schemes))
	for _, scheme := range schemes {
		urls = append(urls, (&url.URL{Scheme: scheme, Host: host, Path: "/v2/"}).String())
	}
	return urls
}

func registryWaitHTTPClient(probeURL string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	parsed, err := url.Parse(probeURL)
	if err == nil && shouldBypassRegistryProxy(parsed.Hostname()) {
		transport.Proxy = nil
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
	}
}

func resolveRegistryHost(registry string) string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.TrimSpace(parsed.Hostname())
		}
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
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
	return strings.Trim(host, "[]")
}

func shouldBypassRegistryProxy(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	switch normalized {
	case "localhost", "127.0.0.1", "registry", "host.docker.internal":
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

// resolveRegistryAddress resolves the host-side registry address for checks from the host machine.
func (w Workflow) resolveRegistryAddress() string {
	// Check HOST_REGISTRY_ADDR first (host-side registry address).
	hostRegistry := strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR"))
	if hostRegistry != "" {
		hostRegistry = strings.TrimPrefix(hostRegistry, "http://")
		hostRegistry = strings.TrimPrefix(hostRegistry, "https://")
		hostRegistry = strings.TrimSuffix(hostRegistry, "/")
		return hostRegistry
	}

	// Use PORT_REGISTRY if set, otherwise default to 5010.
	port := os.Getenv("PORT_REGISTRY")
	if port == "" {
		port = "5010"
	}

	return fmt.Sprintf("127.0.0.1:%s", port)
}
