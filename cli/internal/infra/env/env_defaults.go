// Where: cli/internal/helpers/env_defaults.go
// What: Environment default calculators for ports and networks.
// Why: Keep runtime env setup consistent without Python CLI.
package env

import (
	//nolint:gosec // MD5 is used for deterministic, non-cryptographic hashing.
	"crypto/md5"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"github.com/poruru/edge-serverless-box/meta"
)

var errEnvPrefixRequired = errors.New("ENV_PREFIX is required")

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

// ApplyRuntimeEnv sets all environment variables required for running commands,
// including project metadata, ports, networks, and custom generator parameters.
func ApplyRuntimeEnv(ctx state.Context, resolver func(string) (string, error)) error {
	projectRoot := strings.TrimSpace(ctx.ProjectDir)
	if projectRoot == "" {
		startDir := strings.TrimSpace(ctx.TemplatePath)
		if startDir != "" {
			startDir = filepath.Dir(startDir)
		} else if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		}
		if resolver == nil {
			return fmt.Errorf("project root is required to apply runtime env")
		}
		resolved, err := resolver(startDir)
		if err != nil {
			return fmt.Errorf("resolve repo root: %w", err)
		}
		projectRoot = resolved
	}
	if err := ApplyBrandingEnvWithRoot(projectRoot); err != nil {
		return err
	}
	if err := applyModeEnv(ctx.Mode); err != nil {
		return err
	}

	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		env = "default"
	}

	// Host-level variables (with prefix) for brand-aware logic
	if err := envutil.SetHostEnv(constants.HostSuffixEnv, env); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixEnv, err)
	}
	if err := envutil.SetHostEnv(constants.HostSuffixMode, ctx.Mode); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixMode, err)
	}

	// Compose variables (no prefix) for docker-compose.yml reference
	setEnvIfEmpty("ENV", env)
	setEnvIfEmpty("MODE", ctx.Mode)

	// Compose project metadata
	if os.Getenv(constants.EnvProjectName) == "" {
		_ = os.Setenv(constants.EnvProjectName, ctx.ComposeProject)
	}

	applyPortDefaults(env)
	applySubnetDefaults(env)
	if err := applyRegistryDefaults(ctx.Mode); err != nil {
		return err
	}

	if err := applyConfigDirEnv(ctx, resolver); err != nil {
		return err
	}
	if err := applyProxyDefaults(); err != nil {
		return err
	}
	if err := normalizeRegistryEnv(); err != nil {
		return err
	}
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
	setEnvIfEmpty("BUILDX_BUILDER", fmt.Sprintf("%s-buildx", meta.Slug))
	return nil
}

// ApplyBrandingEnvWithRoot synchronizes branding constants to environment variables,
// using the given project root to build .<brand> asset paths.
func ApplyBrandingEnvWithRoot(projectRoot string) error {
	if strings.TrimSpace(meta.EnvPrefix) == "" {
		return errEnvPrefixRequired
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return fmt.Errorf("project root is required")
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	_ = os.Setenv(constants.EnvRootCAMountID, meta.RootCAMountID)
	setEnvIfEmpty("ROOT_CA_CERT_FILENAME", meta.RootCACertFilename)
	_ = os.Setenv("ENV_PREFIX", meta.EnvPrefix)
	_ = os.Setenv("CLI_CMD", meta.Slug)

	brandDir := filepath.Join(root, meta.HomeDir)
	certDir := strings.TrimSpace(os.Getenv("CERT_DIR"))
	if certDir == "" {
		certDir = filepath.Join(brandDir, "certs")
		setEnvIfEmpty("CERT_DIR", certDir)
	}
	if err := setHostEnvIfEmpty(constants.HostSuffixCertDir, certDir); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixCertDir, err)
	}
	caPath := filepath.Join(certDir, meta.RootCACertFilename)
	if err := setHostEnvIfEmpty(constants.HostSuffixCACertPath, caPath); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixCACertPath, err)
	}
	buildkitConfig := strings.TrimSpace(os.Getenv(constants.EnvBuildkitdConfig))
	if buildkitConfig == "" {
		buildkitConfig = filepath.Join(brandDir, "buildkitd.toml")
		setEnvIfEmpty(constants.EnvBuildkitdConfig, buildkitConfig)
	}

	// Calculate fingerprint for build cache invalidation if CA changes
	if data, err := os.ReadFile(caPath); err == nil {
		//nolint:gosec // MD5 is sufficient for non-cryptographic fingerprinting.
		fp := fmt.Sprintf("%x", md5.Sum(data))
		_ = os.Setenv("ROOT_CA_FINGERPRINT", fp)
	}
	return nil
}

// applyProxyDefaults ensures that proxy-related environment variables are consistent
// and that NO_PROXY includes necessary local targets to avoid connection issues
// in proxy environments. Matches the behavior of the Python E2E runner.
func applyProxyDefaults() error {
	proxyKeys := []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"}
	hasProxy := false
	for _, key := range proxyKeys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			hasProxy = true
			break
		}
	}

	existingNoProxy := os.Getenv("NO_PROXY")
	if existingNoProxy == "" {
		existingNoProxy = os.Getenv("no_proxy")
	}

	extraNoProxy, err := envutil.GetHostEnv(constants.HostSuffixNoProxyExtra)
	if err != nil {
		return fmt.Errorf("get host env %s: %w", constants.HostSuffixNoProxyExtra, err)
	}

	if !hasProxy && existingNoProxy == "" && extraNoProxy == "" {
		return nil
	}

	defaultTargets := []string{
		"agent",
		"database",
		"gateway",
		"local-proxy",
		"localhost",
		"registry",
		"runtime-node",
		"s3-storage",
		"victorialogs",
		"::1",
		"10.88.0.0/16",
		"10.99.0.1",
		"127.0.0.1",
		"172.20.0.0/16",
	}

	split := func(val string) []string {
		if val == "" {
			return nil
		}
		val = strings.ReplaceAll(val, ";", ",")
		parts := strings.Split(val, ",")
		var cleaned []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		return cleaned
	}

	var merged []string
	seen := make(map[string]bool)

	addItem := func(item string) {
		if item != "" && !seen[item] {
			merged = append(merged, item)
			seen[item] = true
		}
	}

	for _, item := range split(existingNoProxy) {
		addItem(item)
	}
	for _, item := range defaultTargets {
		addItem(item)
	}
	for _, item := range split(extraNoProxy) {
		addItem(item)
	}

	if len(merged) > 0 {
		val := strings.Join(merged, ",")
		_ = os.Setenv("NO_PROXY", val)
		_ = os.Setenv("no_proxy", val)
	}

	// Sync upper/lower case versions for subprocesses
	sync := func(upper, lower string) {
		u := os.Getenv(upper)
		l := os.Getenv(lower)
		if u != "" && l == "" {
			_ = os.Setenv(lower, u)
		} else if l != "" && u == "" {
			_ = os.Setenv(upper, l)
		}
	}
	sync("HTTP_PROXY", "http_proxy")
	sync("HTTPS_PROXY", "https_proxy")
	return nil
}

func normalizeRegistryEnv() error {
	key, err := envutil.HostEnvKey(constants.HostSuffixRegistry)
	if err != nil {
		return fmt.Errorf("resolve host env key for registry: %w", err)
	}
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	if !strings.HasSuffix(value, "/") {
		value += "/"
		_ = os.Setenv(key, value)
	}
	return nil
}

// applyPortDefaults sets default port environment variables with an offset
// calculated from a hash of the environment name. Skips already-set variables.
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
	// Default to {project}-external to match docker-compose.yml default
	setEnvIfEmpty(constants.EnvNetworkExternal, fmt.Sprintf("%s-external", os.Getenv(constants.EnvProjectName)))
	setEnvIfEmpty(constants.EnvRuntimeNetSubnet, fmt.Sprintf("172.%d.0.0/16", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvRuntimeNodeIP, fmt.Sprintf("172.%d.0.10", envRuntimeSubnetIndex(env)))
	setEnvIfEmpty(constants.EnvLambdaNetwork, fmt.Sprintf("%s_int_%s", meta.Slug, env))
}

// applyRegistryDefaults sets registry-related defaults for both docker and containerd modes.
func applyRegistryDefaults(mode string) error {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		normalized = "docker"
	}
	containerRegistry := constants.DefaultContainerRegistry
	if normalized == "docker" {
		containerRegistry = constants.DefaultContainerRegistryHost
	}
	if strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)) == "" {
		_ = os.Setenv(constants.EnvContainerRegistry, containerRegistry)
	}
	if strings.TrimSpace(containerRegistry) != "" {
		registry := containerRegistry
		if !strings.HasSuffix(registry, "/") {
			registry += "/"
		}
		if value, err := envutil.GetHostEnv(constants.HostSuffixRegistry); err == nil {
			if strings.TrimSpace(value) == "" {
				if err := envutil.SetHostEnv(constants.HostSuffixRegistry, registry); err != nil {
					return fmt.Errorf("set host env %s: %w", constants.HostSuffixRegistry, err)
				}
			}
		} else {
			return fmt.Errorf("get host env %s: %w", constants.HostSuffixRegistry, err)
		}
	}
	return nil
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

// applyConfigDirEnv sets the CONFIG_DIR environment variable
// based on the discovered project structure.
func applyConfigDirEnv(ctx state.Context, resolver func(string) (string, error)) error {
	_ = resolver

	stagingAbs, err := staging.ConfigDir(ctx.TemplatePath, ctx.ComposeProject, ctx.Env)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	if _, err := os.Stat(stagingAbs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat config dir: %w", err)
	}
	val := filepath.ToSlash(stagingAbs)
	if err := envutil.SetHostEnv(constants.HostSuffixConfigDir, val); err != nil {
		return fmt.Errorf("set host env %s: %w", constants.HostSuffixConfigDir, err)
	}
	setEnvIfEmpty(constants.EnvConfigDir, val)
	return nil
}

// setEnvIfEmpty sets an environment variable only if it's currently empty.
func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}

func setHostEnvIfEmpty(suffix, value string) error {
	key, err := envutil.HostEnvKey(suffix)
	if err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return nil
	}
	return envutil.SetHostEnv(suffix, value)
}
