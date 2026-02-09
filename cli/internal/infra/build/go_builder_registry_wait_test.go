// Where: cli/internal/infra/build/go_builder_registry_wait_test.go
// What: Tests for registry wait proxy behavior.
// Why: Ensure local registry probes do not depend on host proxy routing.
package build

import (
	"net/http"
	"testing"
)

func TestShouldBypassRegistryProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		registry string
		want     bool
	}{
		{name: "registry-service", registry: "registry:5010", want: true},
		{name: "localhost", registry: "localhost:5010", want: true},
		{name: "loopback", registry: "127.0.0.1:5010", want: true},
		{name: "host-docker-internal", registry: "host.docker.internal:5010", want: true},
		{name: "external", registry: "public.ecr.aws", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldBypassRegistryProxy(tt.registry)
			if got != tt.want {
				t.Fatalf("shouldBypassRegistryProxy(%q)=%v, want %v", tt.registry, got, tt.want)
			}
		})
	}
}

func TestRegistryWaitHTTPClientBypassesProxyForLocalRegistry(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	client := registryWaitHTTPClient("registry:5010")
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected nil proxy function for local registry")
	}
}

func TestRegistryWaitHTTPClientUsesProxyForExternalRegistry(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	client := registryWaitHTTPClient("public.ecr.aws")
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("expected proxy function for external registry")
	}
	req, err := http.NewRequest(http.MethodGet, "http://public.ecr.aws/v2/", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("resolve proxy: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("expected proxy url, got nil")
	}
	if got, want := proxyURL.String(), "http://proxy.example:8080"; got != want {
		t.Fatalf("proxyURL=%q, want %q", got, want)
	}
}
