// Where: cli/internal/infra/build/go_builder_ca.go
// What: Root CA path/fingerprint utilities for build cache invalidation.
// Why: Isolate certificate discovery logic from build orchestration.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru-code/esb/cli/internal/constants"
	"github.com/poruru-code/esb/cli/internal/infra/config"
	"github.com/poruru-code/esb/cli/internal/infra/envutil"
	"github.com/poruru-code/esb/cli/internal/meta"
)

func resolveRootCAPath() (string, error) {
	value, err := envutil.GetHostEnv(constants.HostSuffixCACertPath)
	if err != nil {
		return "", err
	}
	if value := strings.TrimSpace(value); value != "" {
		return ensureRootCAPath(expandHome(value))
	}

	value, err = envutil.GetHostEnv(constants.HostSuffixCertDir)
	if err != nil {
		return "", err
	}
	if value := strings.TrimSpace(value); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), meta.RootCACertFilename))
	}

	if value := strings.TrimSpace(os.Getenv("CAROOT")); value != "" {
		return ensureRootCAPath(filepath.Join(expandHome(value), meta.RootCACertFilename))
	}

	startDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("root CA not found: %w", err)
	}
	repoRoot, err := config.ResolveRepoRoot(startDir)
	if err != nil {
		return "", fmt.Errorf("root CA not found: %w", err)
	}
	return ensureRootCAPath(filepath.Join(repoRoot, meta.HomeDir, "certs", meta.RootCACertFilename))
}

func ensureRootCAPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("root CA path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("root CA not found at %s (run mise run setup:certs)", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("root CA path is a directory: %s", path)
	}
	return path, nil
}

func resolveRootCAFingerprint() (string, error) {
	path, err := resolveRootCAPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read root CA: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:4]), nil
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
