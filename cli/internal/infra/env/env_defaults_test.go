// Where: cli/internal/infra/env/env_defaults_test.go
// What: Regression tests for runtime environment default calculation.
// Why: Keep behavior stable while files are split by responsibility.
package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func TestApplyProxyDefaultsMergesNoProxyAndSyncsCase(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8443")
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "example.com")
	t.Setenv("no_proxy", "")
	if err := envutil.SetHostEnv(constants.HostSuffixNoProxyExtra, "custom.local;registry"); err != nil {
		t.Fatalf("set host no_proxy extra: %v", err)
	}

	if err := applyProxyDefaults(); err != nil {
		t.Fatalf("applyProxyDefaults: %v", err)
	}

	merged := os.Getenv("NO_PROXY")
	if merged == "" {
		t.Fatal("NO_PROXY should not be empty")
	}
	if lower := os.Getenv("no_proxy"); lower != merged {
		t.Fatalf("no_proxy=%q, want %q", lower, merged)
	}
	for _, expected := range []string{"example.com", "gateway", "registry", "custom.local"} {
		if !strings.Contains(merged, expected) {
			t.Fatalf("NO_PROXY does not contain %q: %s", expected, merged)
		}
	}
	if got := os.Getenv("http_proxy"); got != "http://proxy.example:8080" {
		t.Fatalf("http_proxy=%q", got)
	}
	if got := os.Getenv("https_proxy"); got != "http://proxy.example:8443" {
		t.Fatalf("https_proxy=%q", got)
	}
}

func TestNormalizeRegistryEnvAddsTrailingSlash(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		t.Fatalf("host env key: %v", err)
	}
	t.Setenv(key, "registry:5010")

	if err := normalizeRegistryEnv(); err != nil {
		t.Fatalf("normalizeRegistryEnv: %v", err)
	}
	if got := os.Getenv(key); got != "registry:5010/" {
		t.Fatalf("registry env=%q", got)
	}
}

func TestApplyPortDefaultsPreservesExistingAndSetsRegistryDefault(t *testing.T) {
	for _, key := range defaultPorts {
		t.Setenv(key, "")
	}
	t.Setenv(constants.EnvPortGatewayHTTP, "8080")

	applyPortDefaults("dev")

	if got := os.Getenv(constants.EnvPortGatewayHTTP); got != "8080" {
		t.Fatalf("PORT_GATEWAY_HTTP=%q", got)
	}
	if got := os.Getenv(constants.EnvPortRegistry); got != "5010" {
		t.Fatalf("PORT_REGISTRY=%q", got)
	}
	if got := os.Getenv(constants.EnvPortS3); got != "0" {
		t.Fatalf("PORT_S3=%q", got)
	}
}

func TestApplySubnetDefaultsForDefaultEnv(t *testing.T) {
	t.Setenv(constants.EnvProjectName, "demo")
	t.Setenv(constants.EnvSubnetExternal, "")
	t.Setenv(constants.EnvNetworkExternal, "")
	t.Setenv(constants.EnvRuntimeNetSubnet, "")
	t.Setenv(constants.EnvRuntimeNodeIP, "")
	t.Setenv(constants.EnvLambdaNetwork, "")

	applySubnetDefaults("default")

	if got := os.Getenv(constants.EnvSubnetExternal); got != "172.50.0.0/16" {
		t.Fatalf("SUBNET_EXTERNAL=%q", got)
	}
	if got := os.Getenv(constants.EnvNetworkExternal); got != "demo-external" {
		t.Fatalf("NETWORK_EXTERNAL=%q", got)
	}
	if got := os.Getenv(constants.EnvRuntimeNetSubnet); got != "172.20.0.0/16" {
		t.Fatalf("RUNTIME_NET_SUBNET=%q", got)
	}
	if got := os.Getenv(constants.EnvRuntimeNodeIP); got != "172.20.0.10" {
		t.Fatalf("RUNTIME_NODE_IP=%q", got)
	}
	wantLambdaNetwork := meta.Slug + "_int_default"
	if got := os.Getenv(constants.EnvLambdaNetwork); got != wantLambdaNetwork {
		t.Fatalf("LAMBDA_NETWORK=%q, want %q", got, wantLambdaNetwork)
	}
}

func TestApplyRegistryDefaultsByMode(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv(constants.EnvContainerRegistry, "")
	if err := envutil.SetHostEnv(constants.HostSuffixRegistry, ""); err != nil {
		t.Fatalf("clear host registry: %v", err)
	}

	if err := applyRegistryDefaults("docker"); err != nil {
		t.Fatalf("docker applyRegistryDefaults: %v", err)
	}
	if got := os.Getenv(constants.EnvContainerRegistry); got != constants.DefaultContainerRegistryHost {
		t.Fatalf("CONTAINER_REGISTRY=%q", got)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixRegistry); err != nil {
		t.Fatalf("read host registry: %v", err)
	} else if got != constants.DefaultContainerRegistryHost+"/" {
		t.Fatalf("host registry=%q", got)
	}
}

func TestApplyConfigDirEnvSetsHostAndComposeVarWhenStagingExists(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv(constants.EnvConfigDir, "")

	projectDir := t.TempDir()
	templatePath := filepath.Join(projectDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	ctx := state.Context{
		TemplatePath:   templatePath,
		ComposeProject: "demo-dev",
		Env:            "dev",
	}
	configDir, err := staging.ConfigDir(templatePath, "demo-dev", "dev")
	if err != nil {
		t.Fatalf("resolve config dir: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	if err := applyConfigDirEnv(ctx, nil); err != nil {
		t.Fatalf("applyConfigDirEnv: %v", err)
	}

	want := filepath.ToSlash(configDir)
	if got := os.Getenv(constants.EnvConfigDir); got != want {
		t.Fatalf("CONFIG_DIR=%q, want %q", got, want)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixConfigDir); err != nil {
		t.Fatalf("get host config dir: %v", err)
	} else if got != want {
		t.Fatalf("host config dir=%q, want %q", got, want)
	}
}
