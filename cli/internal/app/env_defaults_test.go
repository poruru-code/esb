// Where: cli/internal/app/env_defaults_test.go
// What: Tests for environment default helpers.
// Why: Ensure env defaults are applied consistently without overwriting overrides.
package app

import (
	"os"
	"strings"
	"testing"
)

func TestApplyEnvironmentDefaultsSetsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ESB_ENV", "")
	t.Setenv("ESB_PROJECT_NAME", "")
	t.Setenv("ESB_IMAGE_TAG", "")
	t.Setenv("ESB_PORT_GATEWAY_HTTPS", "")
	t.Setenv("ESB_PORT_GATEWAY_HTTP", "")
	t.Setenv("ESB_PORT_AGENT_GRPC", "")
	t.Setenv("ESB_PORT_REGISTRY", "")
	t.Setenv("ESB_SUBNET_EXTERNAL", "")
	t.Setenv("ESB_NETWORK_EXTERNAL", "")
	t.Setenv("RUNTIME_NET_SUBNET", "")
	t.Setenv("RUNTIME_NODE_IP", "")
	t.Setenv("LAMBDA_NETWORK", "")
	t.Setenv("CONTAINER_REGISTRY", "")

	applyEnvironmentDefaults("default", "docker", "esb-default")

	if got := os.Getenv("ESB_PROJECT_NAME"); got != "esb-default" {
		t.Fatalf("unexpected project name: %s", got)
	}
	if got := os.Getenv("ESB_IMAGE_TAG"); got != "default" {
		t.Fatalf("unexpected image tag: %s", got)
	}
	if got := os.Getenv("ESB_PORT_GATEWAY_HTTPS"); got != "443" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv("ESB_PORT_GATEWAY_HTTP"); got != "80" {
		t.Fatalf("unexpected gateway http port: %s", got)
	}
	if got := os.Getenv("ESB_PORT_AGENT_GRPC"); got != "50051" {
		t.Fatalf("unexpected agent grpc port: %s", got)
	}
	if got := os.Getenv("ESB_PORT_REGISTRY"); got != "5010" {
		t.Fatalf("unexpected registry port: %s", got)
	}
	if got := os.Getenv("ESB_SUBNET_EXTERNAL"); got != "172.50.0.0/16" {
		t.Fatalf("unexpected external subnet: %s", got)
	}
	if got := os.Getenv("ESB_NETWORK_EXTERNAL"); got != "esb-default-external" {
		t.Fatalf("unexpected external network: %s", got)
	}
	if got := os.Getenv("RUNTIME_NET_SUBNET"); got != "172.20.0.0/16" {
		t.Fatalf("unexpected runtime subnet: %s", got)
	}
	if got := os.Getenv("RUNTIME_NODE_IP"); got != "172.20.0.10" {
		t.Fatalf("unexpected runtime node ip: %s", got)
	}
	if got := os.Getenv("LAMBDA_NETWORK"); got != "esb_int_default" {
		t.Fatalf("unexpected lambda network: %s", got)
	}
	if got := os.Getenv("CONTAINER_REGISTRY"); got != "" {
		t.Fatalf("unexpected container registry: %s", got)
	}
}

func TestApplyEnvironmentDefaultsDoesNotOverrideExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ESB_ENV", "")
	t.Setenv("ESB_PROJECT_NAME", "custom-project")
	t.Setenv("ESB_IMAGE_TAG", "custom-tag")
	t.Setenv("ESB_PORT_GATEWAY_HTTPS", "1234")
	t.Setenv("ESB_SUBNET_EXTERNAL", "172.99.0.0/16")

	applyEnvironmentDefaults("demo", "docker", "esb-demo")

	if got := os.Getenv("ESB_PROJECT_NAME"); got != "custom-project" {
		t.Fatalf("unexpected project name: %s", got)
	}
	if got := os.Getenv("ESB_IMAGE_TAG"); got != "custom-tag" {
		t.Fatalf("unexpected image tag: %s", got)
	}
	if got := os.Getenv("ESB_PORT_GATEWAY_HTTPS"); got != "1234" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv("ESB_SUBNET_EXTERNAL"); got != "172.99.0.0/16" {
		t.Fatalf("unexpected external subnet: %s", got)
	}
}

func TestApplyEnvironmentDefaultsSetsRegistryForContainerd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ESB_ENV", "")
	t.Setenv("CONTAINER_REGISTRY", "")

	applyEnvironmentDefaults("staging", "containerd", "esb-staging")

	if got := os.Getenv("CONTAINER_REGISTRY"); got != "registry:5010" {
		t.Fatalf("unexpected container registry: %s", got)
	}
}

func TestApplyEnvironmentDefaultsReplacesZeroPorts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ESB_ENV", "")
	t.Setenv("ESB_PORT_GATEWAY_HTTPS", "0")
	t.Setenv("ESB_PORT_DATABASE", "0")

	applyEnvironmentDefaults("default", "docker", "esb-default")

	if got := os.Getenv("ESB_PORT_GATEWAY_HTTPS"); got != "443" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv("ESB_PORT_DATABASE"); got != "8001" {
		t.Fatalf("unexpected database port: %s", got)
	}
}

func TestApplyProxyDefaults(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("NO_PROXY", "existing.com")
	t.Setenv("ESB_NO_PROXY_EXTRA", "extra.com")

	applyProxyDefaults()

	noProxy := os.Getenv("NO_PROXY")
	for _, target := range []string{"existing.com", "agent", "victorialogs", "localhost", "extra.com"} {
		if !containsTarget(noProxy, target) {
			t.Errorf("NO_PROXY missing target: %s (got: %s)", target, noProxy)
		}
	}

	if os.Getenv("http_proxy") != "http://proxy.example.com:8080" {
		t.Errorf("http_proxy not synced")
	}
}

func containsTarget(s, target string) bool {
	parts := strings.Split(s, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == target {
			return true
		}
	}
	return false
}
