package deploy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDefaultRegistryWaiterAcceptsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if err := defaultRegistryWaiter(server.URL, 2*time.Second); err != nil {
		t.Fatalf("defaultRegistryWaiter should accept unauthorized registry response: %v", err)
	}
}

func TestDefaultRegistryWaiterBypassesProxyForLocalRegistry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	if err := defaultRegistryWaiter(parsed.Host, 2*time.Second); err != nil {
		t.Fatalf("defaultRegistryWaiter should bypass proxy for local registry: %v", err)
	}
}

func TestResolveRegistryProbeURLsSchemePriority(t *testing.T) {
	local := resolveRegistryProbeURLs("127.0.0.1:5010")
	if len(local) != 2 {
		t.Fatalf("unexpected local probe urls: %#v", local)
	}
	if got := local[0]; !strings.HasPrefix(got, "http://127.0.0.1:5010/") {
		t.Fatalf("unexpected local first probe url: %s", got)
	}

	external := resolveRegistryProbeURLs("public.ecr.aws")
	if len(external) != 2 {
		t.Fatalf("unexpected external probe urls: %#v", external)
	}
	if got := external[0]; !strings.HasPrefix(got, "https://public.ecr.aws/") {
		t.Fatalf("unexpected external first probe url: %s", got)
	}
}

func TestResolveRegistryAddressTrimsSchemeAndTrailingSlash(t *testing.T) {
	t.Setenv("HOST_REGISTRY_ADDR", "https://registry.example.com:5443/")

	workflow := Workflow{}
	got := workflow.resolveRegistryAddress()
	want := "registry.example.com:5443"
	if got != want {
		t.Fatalf("resolveRegistryAddress() = %q, want %q", got, want)
	}
}
