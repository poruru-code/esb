// Where: cli/internal/infra/env/env_defaults_network.go
// What: Port and subnet default calculators for runtime environment setup.
// Why: Keep network defaults deterministic and local to a single module.
package env

import (
	//nolint:gosec // MD5 is used for deterministic, non-cryptographic hashing.
	"crypto/md5"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/meta"
)

var defaultPorts = []string{
	constants.EnvPortGatewayHTTPS,
	constants.EnvPortGatewayHTTP,
	constants.EnvPortAgentGRPC,
	constants.EnvPortS3,
	constants.EnvPortS3Mgmt,
	constants.EnvPortDatabase,
	constants.EnvPortRegistry,
	constants.EnvPortVictoriaLogs,
}

// applyPortDefaults sets all registered port environment variables to "0" if they
// are currently empty. This enables true dynamic port discovery by Docker Compose.
func applyPortDefaults(_ string) {
	for _, key := range defaultPorts {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			continue
		}
		if key == constants.EnvPortRegistry {
			_ = os.Setenv(key, "5010")
			continue
		}
		_ = os.Setenv(key, "0")
	}
}

// applySubnetDefaults sets default subnet and network environment variables,
// using indices derived from the environment name to avoid collisions.
func applySubnetDefaults(env string) {
	if strings.TrimSpace(os.Getenv(constants.EnvSubnetExternal)) == "" {
		_ = os.Setenv(constants.EnvSubnetExternal, fmt.Sprintf("172.%d.0.0/16", envExternalSubnetIndex(env)))
	}
	// Default to {project}-external to match docker-compose.yml default.
	setEnvIfEmpty(constants.EnvNetworkExternal, fmt.Sprintf("%s-external", os.Getenv(constants.EnvProjectName)))
	setEnvIfEmpty(constants.EnvRuntimeNetSubnet, fmt.Sprintf("172.%d.0.0/16", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvRuntimeNodeIP, fmt.Sprintf("172.%d.0.10", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvLambdaNetwork, fmt.Sprintf("%s_int_%s", meta.Slug, env))
}

// envExternalSubnetIndex returns the third octet for the external subnet.
// Uses 50 for "default", otherwise 60 + hash offset.
func envExternalSubnetIndex(env string) int {
	if env == "default" {
		return 50
	}
	return 60 + hashMod(env, 100)
}

// envRuntimeSubnetIndex returns the third octet for the runtime subnet.
// Uses 20 for "default", otherwise 100 + hash offset.
func envRuntimeSubnetIndex(env string) int {
	if env == "default" {
		return 20
	}
	return 100 + hashMod(env, 100)
}

// hashMod computes a deterministic integer in [0, mod) from a string value
// using MD5 hashing. Used for environment-based offset calculations.
func hashMod(value string, mod int64) int {
	if mod <= 0 {
		return 0
	}
	//nolint:gosec // MD5 is sufficient for deterministic, non-cryptographic hashing.
	sum := md5.Sum([]byte(value))
	hash := new(big.Int).SetBytes(sum[:])
	return int(new(big.Int).Mod(hash, big.NewInt(mod)).Int64())
}
