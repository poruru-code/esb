// Where: cli/internal/helpers/env_defaults.go
// What: Environment default calculators for ports and networks.
// Why: Keep runtime env setup consistent without Python CLI.
package helpers

import (
	"crypto/md5"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/staging"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
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

// applyRuntimeEnv sets all environment variables required for running commands,
// including project metadata, ports, networks, and custom generator parameters.
func applyRuntimeEnv(ctx state.Context, resolver func(string) (string, error)) error {
	if err := ApplyBrandingEnv(); err != nil {
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
		return err
	}
	if err := envutil.SetHostEnv(constants.HostSuffixMode, ctx.Mode); err != nil {
		return err
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

	if err := applyConfigDirEnv(ctx, resolver); err != nil {
		return err
	}
	if err := applyProxyDefaults(); err != nil {
		return err
	}
	if err := normalizeRegistryEnv(); err != nil {
		return err
	}
	applyBuildMetadata()
	if os.Getenv("DOCKER_BUILDKIT") == "" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
	return nil
}

// ApplyBrandingEnv synchronizes branding constants from the meta package
// to environment variables used by Docker Compose and scripts.
func ApplyBrandingEnv() error {
	if strings.TrimSpace(meta.EnvPrefix) == "" {
		return fmt.Errorf("ENV_PREFIX is required")
	}
	_ = os.Setenv(constants.EnvRootCAMountID, meta.RootCAMountID)
	setEnvIfEmpty("ROOT_CA_CERT_FILENAME", meta.RootCACertFilename)
	_ = os.Setenv("ENV_PREFIX", meta.EnvPrefix)
	_ = os.Setenv("CLI_CMD", meta.Slug)

	homeDirName := meta.HomeDir
	if !strings.HasPrefix(homeDirName, ".") {
		homeDirName = "." + homeDirName
	}
	home := os.Getenv("HOME")
	certDir := filepath.Join(home, homeDirName, "certs")
	setEnvIfEmpty("CERT_DIR", certDir)

	// Calculate fingerprint for build cache invalidation if CA changes
	caPath := filepath.Join(certDir, "rootCA.crt")
	if data, err := os.ReadFile(caPath); err == nil {
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
		return err
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
		return err
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
	setEnvIfEmpty(constants.EnvLambdaNetwork, fmt.Sprintf("esb_int_%s", env))
}

// applyRegistryDefaults sets the CONTAINER_REGISTRY environment variable
// for containerd mode when not already specified.
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

// applyConfigDirEnv sets the CONFIG_DIR environment variable
// based on the discovered project structure.
func applyConfigDirEnv(ctx state.Context, resolver func(string) (string, error)) error {
	_ = resolver

	stagingAbs := staging.ConfigDir(ctx.ComposeProject, ctx.Env)
	if _, err := os.Stat(stagingAbs); err != nil {
		return nil
	}
	val := filepath.ToSlash(stagingAbs)
	if err := envutil.SetHostEnv(constants.HostSuffixConfigDir, val); err != nil {
		return err
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

func applyBuildMetadata() {
	if strings.TrimSpace(os.Getenv("GIT_SHA")) == "" {
		_ = os.Setenv("GIT_SHA", resolveGitSHA())
	}
	if strings.TrimSpace(os.Getenv("BUILD_DATE")) == "" {
		_ = os.Setenv("BUILD_DATE", time.Now().UTC().Format(time.RFC3339))
	}
}

func resolveGitSHA() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	val := strings.TrimSpace(string(output))
	if val == "" {
		return "unknown"
	}
	return val
}
