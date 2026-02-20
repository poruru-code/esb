// Where: cli/internal/infra/env/env_defaults_contract_test.go
// What: Contract tests shared with E2E runner for subnet/network defaults.
// Why: Prevent Go/Python drift by validating against a single fixture file.
package env

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/poruru-code/esb/cli/internal/constants"
	"gopkg.in/yaml.v3"
)

type runtimeEnvContract struct {
	Cases []runtimeEnvContractCase `yaml:"cases"`
}

type runtimeEnvContractCase struct {
	Env                 string `yaml:"env"`
	ExternalSubnetIndex int    `yaml:"external_subnet_index"`
	RuntimeSubnetIndex  int    `yaml:"runtime_subnet_index"`
	SubnetExternal      string `yaml:"subnet_external"`
	RuntimeNetSubnet    string `yaml:"runtime_net_subnet"`
	RuntimeNodeIP       string `yaml:"runtime_node_ip"`
	LambdaNetwork       string `yaml:"lambda_network"`
}

func TestRuntimeEnvSubnetIndexContract(t *testing.T) {
	contract := loadRuntimeEnvContract(t)
	for _, tc := range contract.Cases {
		tc := tc
		t.Run(tc.Env, func(t *testing.T) {
			if got := envExternalSubnetIndex(tc.Env); got != tc.ExternalSubnetIndex {
				t.Fatalf("envExternalSubnetIndex(%q)=%d, want %d", tc.Env, got, tc.ExternalSubnetIndex)
			}
			if got := envRuntimeSubnetIndex(tc.Env); got != tc.RuntimeSubnetIndex {
				t.Fatalf("envRuntimeSubnetIndex(%q)=%d, want %d", tc.Env, got, tc.RuntimeSubnetIndex)
			}
		})
	}
}

func TestApplySubnetDefaultsMatchesContract(t *testing.T) {
	contract := loadRuntimeEnvContract(t)
	for _, tc := range contract.Cases {
		tc := tc
		t.Run(tc.Env, func(t *testing.T) {
			t.Setenv(constants.EnvProjectName, "esb-"+tc.Env)
			t.Setenv(constants.EnvSubnetExternal, "")
			t.Setenv(constants.EnvNetworkExternal, "")
			t.Setenv(constants.EnvRuntimeNetSubnet, "")
			t.Setenv(constants.EnvRuntimeNodeIP, "")
			t.Setenv(constants.EnvLambdaNetwork, "")

			applySubnetDefaults(tc.Env)

			if got := os.Getenv(constants.EnvSubnetExternal); got != tc.SubnetExternal {
				t.Fatalf("SUBNET_EXTERNAL=%q, want %q", got, tc.SubnetExternal)
			}
			if got := os.Getenv(constants.EnvRuntimeNetSubnet); got != tc.RuntimeNetSubnet {
				t.Fatalf("RUNTIME_NET_SUBNET=%q, want %q", got, tc.RuntimeNetSubnet)
			}
			if got := os.Getenv(constants.EnvRuntimeNodeIP); got != tc.RuntimeNodeIP {
				t.Fatalf("RUNTIME_NODE_IP=%q, want %q", got, tc.RuntimeNodeIP)
			}
			if got := os.Getenv(constants.EnvLambdaNetwork); got != tc.LambdaNetwork {
				t.Fatalf("LAMBDA_NETWORK=%q, want %q", got, tc.LambdaNetwork)
			}
		})
	}
}

func loadRuntimeEnvContract(t *testing.T) runtimeEnvContract {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	contractPath := filepath.Clean(filepath.Join(
		filepath.Dir(thisFile),
		"..", "..", "..", "..",
		"e2e", "contracts", "runtime_env_contract.yaml",
	))
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read contract fixture: %v", err)
	}
	var contract runtimeEnvContract
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatalf("unmarshal contract fixture: %v", err)
	}
	if len(contract.Cases) == 0 {
		t.Fatal("contract fixture has no cases")
	}
	return contract
}
