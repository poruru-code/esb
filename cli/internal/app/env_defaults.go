// Where: cli/internal/app/env_defaults.go
// What: Environment default calculators for ports and networks.
// Why: Keep runtime env setup consistent without Python CLI.
package app

import (
	"crypto/md5"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
)

var defaultPorts = map[string]int{
	"ESB_PORT_GATEWAY_HTTPS": 443,
	"ESB_PORT_GATEWAY_HTTP":  80,
	"ESB_PORT_AGENT_GRPC":    50051,
	"ESB_PORT_STORAGE":       9000,
	"ESB_PORT_STORAGE_MGMT":  9001,
	"ESB_PORT_DATABASE":      8001,
	"ESB_PORT_REGISTRY":      5010,
	"ESB_PORT_VICTORIALOGS":  9428,
}

func applyEnvironmentDefaults(envName, mode string) {
	env := strings.TrimSpace(envName)
	if env == "" {
		env = "default"
	}

	setEnvIfEmpty("ESB_PROJECT_NAME", fmt.Sprintf("esb-%s", strings.ToLower(env)))
	setEnvIfEmpty("ESB_IMAGE_TAG", env)

	applyPortDefaults(env)
	applySubnetDefaults(env)
	applyRegistryDefaults(mode)
}

func applyPortDefaults(env string) {
	offset := envPortOffset(env)
	for key, base := range defaultPorts {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" && value != "0" {
			continue
		}
		port := base + offset
		_ = os.Setenv(key, strconv.Itoa(port))
	}
}

func applySubnetDefaults(env string) {
	if strings.TrimSpace(os.Getenv("ESB_SUBNET_EXTERNAL")) == "" {
		_ = os.Setenv("ESB_SUBNET_EXTERNAL", fmt.Sprintf("172.%d.0.0/16", envExternalSubnetIndex(env)))
	}
	setEnvIfEmpty("ESB_NETWORK_EXTERNAL", fmt.Sprintf("esb_ext_%s", env))
	setEnvIfEmpty("RUNTIME_NET_SUBNET", fmt.Sprintf("172.%d.0.0/16", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty("RUNTIME_NODE_IP", fmt.Sprintf("172.%d.0.10", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty("LAMBDA_NETWORK", fmt.Sprintf("esb_int_%s", env))
}

func applyRegistryDefaults(mode string) {
	if strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")) != "" {
		return
	}
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(os.Getenv("ESB_MODE")))
	}
	switch normalized {
	case "containerd", "firecracker":
		_ = os.Setenv("CONTAINER_REGISTRY", "registry:5010")
	}
}

func envPortOffset(env string) int {
	if env == "default" {
		return 0
	}
	offset := hashMod(env, 50) * 100
	if offset == 0 {
		offset = 1000
	}
	return offset
}

func envExternalSubnetIndex(env string) int {
	if env == "default" {
		return 50
	}
	return 60 + hashMod(env, 100)
}

func envRuntimeSubnetIndex(env string) int {
	if env == "default" {
		return 20
	}
	return 100 + hashMod(env, 100)
}

func hashMod(value string, mod int64) int {
	if mod <= 0 {
		return 0
	}
	sum := md5.Sum([]byte(value))
	hash := new(big.Int).SetBytes(sum[:])
	return int(new(big.Int).Mod(hash, big.NewInt(mod)).Int64())
}

func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}
