// Where: cli/internal/app/env_defaults.go
// What: Environment default calculators for ports and networks.
// Why: Keep runtime env setup consistent without Python CLI.
package app

import (
	"crypto/md5"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
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

// applyRuntimeEnv sets all environment variables required for running commands,
// including project metadata, ports, networks, and custom generator parameters.
func applyRuntimeEnv(ctx state.Context) {
	applyModeEnv(ctx.Mode)

	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		env = "default"
	}

	_ = os.Setenv("ESB_ENV", env)
	setEnvIfEmpty("ESB_PROJECT_NAME", ctx.ComposeProject)
	setEnvIfEmpty("ESB_IMAGE_TAG", env)

	applyPortDefaults(env)
	applySubnetDefaults(env)
	applyRegistryDefaults(ctx.Mode)

	_ = applyGeneratorConfigEnv(ctx.GeneratorPath)
	applyConfigDirEnv(ctx)
}

// applyEnvironmentDefaults is a legacy helper. New code should use applyRuntimeEnv.
func applyEnvironmentDefaults(envName, mode, composeProject string) {
	applyRuntimeEnv(state.Context{
		Env:            envName,
		Mode:           mode,
		ComposeProject: composeProject,
	})
}

// applyPortDefaults sets default port environment variables with an offset
// calculated from a hash of the environment name. Skips already-set variables.
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

// applySubnetDefaults sets default subnet and network environment variables,
// using indices derived from the environment name to avoid collisions.
func applySubnetDefaults(env string) {
	if strings.TrimSpace(os.Getenv("ESB_SUBNET_EXTERNAL")) == "" {
		_ = os.Setenv("ESB_SUBNET_EXTERNAL", fmt.Sprintf("172.%d.0.0/16", envExternalSubnetIndex(env)))
	}
	setEnvIfEmpty("ESB_NETWORK_EXTERNAL", fmt.Sprintf("esb_ext_%s", env))
	setEnvIfEmpty("RUNTIME_NET_SUBNET", fmt.Sprintf("172.%d.0.0/16", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty("RUNTIME_NODE_IP", fmt.Sprintf("172.%d.0.10", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty("LAMBDA_NETWORK", fmt.Sprintf("esb_int_%s", env))
}

// applyRegistryDefaults sets the CONTAINER_REGISTRY environment variable
// for containerd/firecracker modes when not already specified.
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

// envPortOffset calculates a port offset for the given environment name.
// Returns 0 for "default", otherwise a hash-based offset in hundreds.
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
	sum := md5.Sum([]byte(value))
	hash := new(big.Int).SetBytes(sum[:])
	return int(new(big.Int).Mod(hash, big.NewInt(mod)).Int64())
}

// applyGeneratorConfigEnv reads the generator.yml configuration and sets
// environment variables for function/routing paths and custom parameters.
func applyGeneratorConfigEnv(generatorPath string) error {
	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Paths.FunctionsYml) != "" {
		_ = os.Setenv("GATEWAY_FUNCTIONS_YML", cfg.Paths.FunctionsYml)
	}
	if strings.TrimSpace(cfg.Paths.RoutingYml) != "" {
		_ = os.Setenv("GATEWAY_ROUTING_YML", cfg.Paths.RoutingYml)
	}

	for key, value := range cfg.Parameters {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		switch v := value.(type) {
		case string, bool, int, int64, int32, float64, float32, uint, uint64, uint32:
			_ = os.Setenv(key, fmt.Sprint(v))
		}
	}
	return nil
}

// applyConfigDirEnv sets the ESB_CONFIG_DIR environment variable
// based on the discovered project structure.
func applyConfigDirEnv(ctx state.Context) {
	if strings.TrimSpace(os.Getenv("ESB_CONFIG_DIR")) != "" {
		return
	}

	root, err := config.ResolveRepoRoot(ctx.ProjectDir)
	if err != nil {
		return
	}
	stagingRel := filepath.Join("services", "gateway", ".esb-staging", ctx.Env, "config")
	stagingAbs := filepath.Join(root, stagingRel)
	if _, err := os.Stat(stagingAbs); err != nil {
		return
	}
	_ = os.Setenv("ESB_CONFIG_DIR", filepath.ToSlash(stagingRel))
}

// setEnvIfEmpty sets an environment variable only if it's currently empty.
func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}
