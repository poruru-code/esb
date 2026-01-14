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
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

var defaultPorts = map[string]int{
	constants.EnvPortGatewayHTTPS: 443,
	constants.EnvPortGatewayHTTP:  80,
	constants.EnvPortAgentCGRPC:   50051,
	constants.EnvPortS3:           9000,
	constants.EnvPortS3Mgmt:       9001,
	constants.EnvPortDatabase:     8001,
	constants.EnvPortRegistry:     5010,
	constants.EnvPortVictoriaLogs: 9428,
}

// applyRuntimeEnv sets all environment variables required for running commands,
// including project metadata, ports, networks, and custom generator parameters.
func applyRuntimeEnv(ctx state.Context, resolver func(string) (string, error)) {
	applyModeEnv(ctx.Mode)

	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		env = "default"
	}

	_ = os.Setenv(constants.EnvESBEnv, env)
	setEnvIfEmpty(constants.EnvESBProjectName, ctx.ComposeProject)
	setEnvIfEmpty(constants.EnvESBImageTag, env)

	applyPortDefaults(env)
	applySubnetDefaults(env)
	applyRegistryDefaults(ctx.Mode)

	_ = applyGeneratorConfigEnv(ctx.GeneratorPath)
	applyConfigDirEnv(ctx, resolver)
}

// applyEnvironmentDefaults is a legacy helper. New code should use applyRuntimeEnv.
func applyEnvironmentDefaults(envName, mode, composeProject string) {
	applyRuntimeEnv(state.Context{
		Env:            envName,
		Mode:           mode,
		ComposeProject: composeProject,
	}, config.ResolveRepoRoot)
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
	if strings.TrimSpace(os.Getenv(constants.EnvSubnetExternal)) == "" {
		_ = os.Setenv(constants.EnvSubnetExternal, fmt.Sprintf("172.%d.0.0/16", envExternalSubnetIndex(env)))
	}
	// Default to {project}-external to match docker-compose.yml default
	setEnvIfEmpty(constants.EnvNetworkExternal, fmt.Sprintf("%s-external", os.Getenv(constants.EnvESBProjectName)))
	setEnvIfEmpty(constants.EnvRuntimeNetSubnet, fmt.Sprintf("172.%d.0.0/16", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvRuntimeNodeIP, fmt.Sprintf("172.%d.0.10", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvLambdaNetwork, fmt.Sprintf("esb_int_%s", env))
}

// applyRegistryDefaults sets the CONTAINER_REGISTRY environment variable
// for containerd/firecracker modes when not already specified.
func applyRegistryDefaults(mode string) {
	if strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)) != "" {
		return
	}
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(os.Getenv(constants.EnvESBMode)))
	}
	switch normalized {
	case "containerd", "firecracker":
		_ = os.Setenv(constants.EnvContainerRegistry, "registry:5010")
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
		_ = os.Setenv(constants.EnvGatewayFunctionsYml, cfg.Paths.FunctionsYml)
	}
	if strings.TrimSpace(cfg.Paths.RoutingYml) != "" {
		_ = os.Setenv(constants.EnvGatewayRoutingYml, cfg.Paths.RoutingYml)
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
func applyConfigDirEnv(ctx state.Context, resolver func(string) (string, error)) {
	if strings.TrimSpace(os.Getenv(constants.EnvESBConfigDir)) != "" {
		return
	}

	if resolver == nil {
		resolver = config.ResolveRepoRoot
	}

	root, err := resolver(ctx.ProjectDir)
	if err != nil {
		return
	}
	stagingRel := filepath.Join("services", "gateway", ".esb-staging", ctx.Env, "config")
	stagingAbs := filepath.Join(root, stagingRel)
	if _, err := os.Stat(stagingAbs); err != nil {
		return
	}
	_ = os.Setenv(constants.EnvESBConfigDir, filepath.ToSlash(stagingRel))
}

// setEnvIfEmpty sets an environment variable only if it's currently empty.
func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}
