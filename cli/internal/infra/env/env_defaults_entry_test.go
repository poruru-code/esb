package env

import (
	"errors"
	"fmt"
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

func TestApplyRuntimeEnvRequiresResolverWhenProjectRootUnknown(t *testing.T) {
	err := ApplyRuntimeEnv(state.Context{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "project root is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyRuntimeEnvPropagatesResolverError(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "template.yaml")
	err := ApplyRuntimeEnv(
		state.Context{
			TemplatePath: templatePath,
		},
		func(string) (string, error) {
			return "", errors.New("resolver boom")
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resolve repo root: resolver boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyRuntimeEnvSetsCoreDefaults(t *testing.T) {
	projectRoot := t.TempDir()
	setWorkingDir(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "docker-compose.docker.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(projectRoot, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	certDir := filepath.Join(projectRoot, meta.HomeDir, "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	caPath := filepath.Join(certDir, meta.RootCACertFilename)
	if err := os.WriteFile(caPath, []byte("root-ca"), 0o600); err != nil {
		t.Fatalf("write root ca: %v", err)
	}

	t.Setenv("ENV_PREFIX", "")
	t.Setenv(constants.EnvConfigDir, "")
	t.Setenv(constants.EnvContainerRegistry, "")
	t.Setenv(constants.EnvProjectName, "")
	t.Setenv("DOCKER_BUILDKIT", "")
	t.Setenv("BUILDX_BUILDER", "")

	resolverCalled := false
	err := ApplyRuntimeEnv(
		state.Context{
			ProjectDir:     "",
			TemplatePath:   templatePath,
			ComposeProject: "esb-dev",
			Mode:           "docker",
			Env:            "",
		},
		func(start string) (string, error) {
			resolverCalled = true
			if filepath.Clean(start) != filepath.Clean(filepath.Dir(templatePath)) {
				return "", fmt.Errorf("unexpected resolver start dir: %s", start)
			}
			return projectRoot, nil
		},
	)
	if err != nil {
		t.Fatalf("ApplyRuntimeEnv() error = %v", err)
	}
	if !resolverCalled {
		t.Fatal("resolver was not called")
	}

	if got := os.Getenv("ENV"); got != "default" {
		t.Fatalf("ENV=%q, want default", got)
	}
	if got := os.Getenv("MODE"); got != "docker" {
		t.Fatalf("MODE=%q, want docker", got)
	}
	if got := os.Getenv(constants.EnvProjectName); got != "esb-dev" {
		t.Fatalf("PROJECT_NAME=%q, want esb-dev", got)
	}
	if got := os.Getenv(constants.EnvContainerRegistry); got != constants.DefaultContainerRegistryHost {
		t.Fatalf("CONTAINER_REGISTRY=%q, want %q", got, constants.DefaultContainerRegistryHost)
	}
	if got := os.Getenv("DOCKER_BUILDKIT"); got != "1" {
		t.Fatalf("DOCKER_BUILDKIT=%q, want 1", got)
	}
	if got := os.Getenv("BUILDX_BUILDER"); got != meta.Slug+"-buildx" {
		t.Fatalf("BUILDX_BUILDER=%q", got)
	}

	wantConfig, err := staging.ConfigDir(templatePath, "esb-dev", "default")
	if err != nil {
		t.Fatalf("resolve config dir: %v", err)
	}
	wantConfig = filepath.ToSlash(wantConfig)
	if got := os.Getenv(constants.EnvConfigDir); got != wantConfig {
		t.Fatalf("CONFIG_DIR=%q, want %q", got, wantConfig)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixEnv); err != nil {
		t.Fatalf("read host ENV: %v", err)
	} else if got != "default" {
		t.Fatalf("host ENV=%q, want default", got)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixMode); err != nil {
		t.Fatalf("read host MODE: %v", err)
	} else if got != "docker" {
		t.Fatalf("host MODE=%q, want docker", got)
	}
	if got, err := envutil.GetHostEnv(constants.HostSuffixConfigDir); err != nil {
		t.Fatalf("read host CONFIG_DIR: %v", err)
	} else if got != wantConfig {
		t.Fatalf("host CONFIG_DIR=%q, want %q", got, wantConfig)
	}
}

func TestApplyBrandingEnvWithRoot(t *testing.T) {
	t.Run("rejects empty root", func(t *testing.T) {
		if err := ApplyBrandingEnvWithRoot(""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("sets default cert paths and fingerprint", func(t *testing.T) {
		root := t.TempDir()
		certDir := filepath.Join(root, meta.HomeDir, "certs")
		if err := os.MkdirAll(certDir, 0o755); err != nil {
			t.Fatalf("mkdir cert dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(certDir, meta.RootCACertFilename), []byte("abc"), 0o600); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		t.Setenv("ENV_PREFIX", "")
		t.Setenv("CERT_DIR", "")
		t.Setenv("ROOT_CA_FINGERPRINT", "")
		t.Setenv("ESB_CERT_DIR", "")
		t.Setenv("ESB_CA_CERT_PATH", "")

		if err := ApplyBrandingEnvWithRoot(root); err != nil {
			t.Fatalf("ApplyBrandingEnvWithRoot() error = %v", err)
		}
		if got := os.Getenv("ENV_PREFIX"); got != meta.EnvPrefix {
			t.Fatalf("ENV_PREFIX=%q, want %q", got, meta.EnvPrefix)
		}
		if got := os.Getenv("CERT_DIR"); got != certDir {
			t.Fatalf("CERT_DIR=%q, want %q", got, certDir)
		}
		if got := os.Getenv("ROOT_CA_FINGERPRINT"); got != "900150983cd24fb0d6963f7d28e17f72" {
			t.Fatalf("ROOT_CA_FINGERPRINT=%q", got)
		}
		if got, err := envutil.GetHostEnv(constants.HostSuffixCertDir); err != nil {
			t.Fatalf("read host CERT_DIR: %v", err)
		} else if got != certDir {
			t.Fatalf("host CERT_DIR=%q, want %q", got, certDir)
		}
	})
}

func TestApplyModeEnv(t *testing.T) {
	t.Run("no-op for empty mode", func(t *testing.T) {
		t.Setenv("ENV_PREFIX", "ESB")
		if err := applyModeEnv(""); err != nil {
			t.Fatalf("applyModeEnv() error = %v", err)
		}
	})

	t.Run("sets lowercase host mode when empty", func(t *testing.T) {
		t.Setenv("ENV_PREFIX", "ESB")
		t.Setenv("ESB_MODE", "")
		if err := applyModeEnv("Docker"); err != nil {
			t.Fatalf("applyModeEnv() error = %v", err)
		}
		if got := os.Getenv("ESB_MODE"); got != "docker" {
			t.Fatalf("ESB_MODE=%q, want docker", got)
		}
	})

	t.Run("keeps existing host mode", func(t *testing.T) {
		t.Setenv("ENV_PREFIX", "ESB")
		t.Setenv("ESB_MODE", "containerd")
		if err := applyModeEnv("docker"); err != nil {
			t.Fatalf("applyModeEnv() error = %v", err)
		}
		if got := os.Getenv("ESB_MODE"); got != "containerd" {
			t.Fatalf("ESB_MODE=%q, want containerd", got)
		}
	})

	t.Run("fails when host prefix missing", func(t *testing.T) {
		t.Setenv("ENV_PREFIX", "")
		err := applyModeEnv("docker")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "get host env MODE") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSetHostEnvIfEmpty(t *testing.T) {
	t.Setenv("ENV_PREFIX", "ESB")
	t.Setenv("ESB_TAG", "")
	if err := setHostEnvIfEmpty(constants.HostSuffixTag, "v1.0.0"); err != nil {
		t.Fatalf("setHostEnvIfEmpty() error = %v", err)
	}
	if got := os.Getenv("ESB_TAG"); got != "v1.0.0" {
		t.Fatalf("ESB_TAG=%q, want v1.0.0", got)
	}

	t.Setenv("ESB_TAG", "keep")
	if err := setHostEnvIfEmpty(constants.HostSuffixTag, "override"); err != nil {
		t.Fatalf("setHostEnvIfEmpty() error = %v", err)
	}
	if got := os.Getenv("ESB_TAG"); got != "keep" {
		t.Fatalf("ESB_TAG=%q, want keep", got)
	}
}
