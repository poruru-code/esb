// Where: cli/internal/app/env_defaults_test.go
// What: Tests for environment default helpers.
// Why: Ensure env defaults are applied consistently without overwriting overrides.
package app

import (
	"os"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
)

func TestApplyEnvironmentDefaultsSetsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(constants.EnvESBEnv, "")
	t.Setenv(constants.EnvESBProjectName, "")
	t.Setenv(constants.EnvESBImageTag, "")
	t.Setenv(constants.EnvPortGatewayHTTPS, "")
	t.Setenv(constants.EnvPortGatewayHTTP, "")
	t.Setenv(constants.EnvPortAgentCGRPC, "")
	t.Setenv(constants.EnvPortRegistry, "")
	t.Setenv(constants.EnvSubnetExternal, "")
	t.Setenv(constants.EnvNetworkExternal, "")
	t.Setenv(constants.EnvRuntimeNetSubnet, "")
	t.Setenv(constants.EnvRuntimeNodeIP, "")
	t.Setenv(constants.EnvLambdaNetwork, "")
	t.Setenv(constants.EnvContainerRegistry, "")

	applyEnvironmentDefaults("default", "docker", constants.BrandingSlug+"-default")

	if got := os.Getenv(constants.EnvESBProjectName); got != constants.BrandingSlug+"-default" {
		t.Fatalf("unexpected project name: %s", got)
	}
	if got := os.Getenv(constants.EnvESBImageTag); got != "docker" {
		t.Fatalf("unexpected image tag: %s", got)
	}
	if got := os.Getenv(constants.EnvPortGatewayHTTPS); got != "443" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv(constants.EnvPortGatewayHTTP); got != "80" {
		t.Fatalf("unexpected gateway http port: %s", got)
	}
	if got := os.Getenv(constants.EnvPortAgentCGRPC); got != "50051" {
		t.Fatalf("unexpected agent grpc port: %s", got)
	}
	if got := os.Getenv(constants.EnvPortRegistry); got != "5010" {
		t.Fatalf("unexpected registry port: %s", got)
	}
	if got := os.Getenv(constants.EnvSubnetExternal); got != "172.50.0.0/16" {
		t.Fatalf("unexpected external subnet: %s", got)
	}
	if got := os.Getenv(constants.EnvNetworkExternal); got != constants.BrandingSlug+"-default-external" {
		t.Fatalf("unexpected external network: %s", got)
	}
	if got := os.Getenv(constants.EnvRuntimeNetSubnet); got != "172.20.0.0/16" {
		t.Fatalf("unexpected runtime subnet: %s", got)
	}
	if got := os.Getenv(constants.EnvRuntimeNodeIP); got != "172.20.0.10" {
		t.Fatalf("unexpected runtime node ip: %s", got)
	}
	if got := os.Getenv(constants.EnvLambdaNetwork); got != constants.BrandingSlug+"_int_default" {
		t.Fatalf("unexpected lambda network: %s", got)
	}
	if got := os.Getenv(constants.EnvContainerRegistry); got != "" {
		t.Fatalf("unexpected container registry: %s", got)
	}
}

func TestApplyEnvironmentDefaultsDoesNotOverrideExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(constants.EnvESBEnv, "")
	t.Setenv(constants.EnvESBProjectName, "custom-project")
	t.Setenv(constants.EnvESBImageTag, "custom-tag")
	t.Setenv(constants.EnvPortGatewayHTTPS, "1234")
	t.Setenv(constants.EnvSubnetExternal, "172.99.0.0/16")

	applyEnvironmentDefaults("demo", "docker", constants.BrandingSlug+"-demo")

	if got := os.Getenv(constants.EnvESBProjectName); got != "custom-project" {
		t.Fatalf("unexpected project name: %s", got)
	}
	if got := os.Getenv(constants.EnvESBImageTag); got != "custom-tag" {
		t.Fatalf("unexpected image tag: %s", got)
	}
	if got := os.Getenv(constants.EnvPortGatewayHTTPS); got != "1234" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv(constants.EnvSubnetExternal); got != "172.99.0.0/16" {
		t.Fatalf("unexpected external subnet: %s", got)
	}
}

func TestApplyEnvironmentDefaultsSetsRegistryForContainerd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(constants.EnvESBEnv, "")
	t.Setenv(constants.EnvContainerRegistry, "")

	applyEnvironmentDefaults("staging", "containerd", constants.BrandingSlug+"-staging")

	if got := os.Getenv(constants.EnvContainerRegistry); got != "registry:5010" {
		t.Fatalf("unexpected container registry: %s", got)
	}
}

func TestApplyEnvironmentDefaultsReplacesZeroPorts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(constants.EnvESBEnv, "")
	t.Setenv(constants.EnvPortGatewayHTTPS, "0")
	t.Setenv(constants.EnvPortDatabase, "0")

	applyEnvironmentDefaults("default", "docker", constants.BrandingSlug+"-default")

	if got := os.Getenv(constants.EnvPortGatewayHTTPS); got != "443" {
		t.Fatalf("unexpected gateway https port: %s", got)
	}
	if got := os.Getenv(constants.EnvPortDatabase); got != "8001" {
		t.Fatalf("unexpected database port: %s", got)
	}
}

func TestApplyProxyDefaults(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("NO_PROXY", "existing.com")
	t.Setenv(constants.EnvESBNoProxyExtra, "extra.com")

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
