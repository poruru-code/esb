// Where: cli/internal/infra/env/env_defaults_branding.go
// What: Branding-related runtime environment defaults.
// Why: Keep brand path/CA defaults independent from deploy flow wiring.
package env

import (
	//nolint:gosec // MD5 is used for deterministic, non-cryptographic hashing.
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/meta"
)

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

	// Calculate fingerprint for build cache invalidation if CA changes.
	if data, err := os.ReadFile(caPath); err == nil {
		//nolint:gosec // MD5 is sufficient for non-cryptographic fingerprinting.
		fp := fmt.Sprintf("%x", md5.Sum(data))
		_ = os.Setenv("ROOT_CA_FINGERPRINT", fp)
	}
	return nil
}
