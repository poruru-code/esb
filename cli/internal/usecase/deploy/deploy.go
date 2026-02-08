// Where: cli/internal/usecase/deploy/deploy.go
// What: Deploy workflow orchestration.
// Why: Encapsulate deploy-specific logic without CLI concerns.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
)

var (
	errBuilderNotConfigured       = errors.New("builder is not configured")
	errComposeRunnerNotConfigured = errors.New("compose runner is not configured")
	errUnsupportedDockerClient    = errors.New("unsupported docker client")
	errRegistryNotResponding      = errors.New("registry not responding")
)

// Request captures the inputs required to run a deploy.
type Request struct {
	Context        state.Context
	Env            string
	TemplatePath   string
	Mode           string
	OutputDir      string
	Parameters     map[string]string
	Tag            string
	NoCache        bool
	NoDeps         bool
	Verbose        bool
	ComposeFiles   []string
	BuildOnly      bool
	BundleManifest bool
	ImagePrewarm   string
	Emoji          bool
}

// RegistryWaiter checks registry readiness.
type RegistryWaiter func(registry string, timeout time.Duration) error

func defaultRegistryWaiter(registry string, timeout time.Duration) error {
	if registry == "" {
		return nil
	}

	url := fmt.Sprintf("http://%s/v2/", registry)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create registry request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			// Only accept 200 OK as success (not 401/404/etc)
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%w at %s", errRegistryNotResponding, url)
}

// Workflow executes the deploy orchestration steps.
type Workflow struct {
	Build           func(build.BuildRequest) error
	ApplyRuntimeEnv func(state.Context) error
	UserInterface   ui.UserInterface
	ComposeRunner   compose.CommandRunner
	RegistryWaiter  RegistryWaiter
}

// NewDeployWorkflow constructs a Workflow.
func NewDeployWorkflow(
	build func(build.BuildRequest) error,
	applyRuntimeEnv func(state.Context) error,
	ui ui.UserInterface,
	composeRunner compose.CommandRunner,
) Workflow {
	return Workflow{
		Build:           build,
		ApplyRuntimeEnv: applyRuntimeEnv,
		UserInterface:   ui,
		ComposeRunner:   composeRunner,
		RegistryWaiter:  defaultRegistryWaiter,
	}
}

// Run executes the deploy workflow.
func (w Workflow) Run(req Request) error {
	if w.Build == nil {
		return errBuilderNotConfigured
	}
	if w.ComposeRunner == nil {
		return errComposeRunnerNotConfigured
	}
	imagePrewarm, err := normalizeImagePrewarmMode(req.ImagePrewarm)
	if err != nil {
		return err
	}

	req = w.alignGatewayRuntime(req)

	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	if !req.BuildOnly {
		// Wait for registry to be ready
		registry := w.resolveRegistryAddress()
		if w.RegistryWaiter != nil {
			if err := w.RegistryWaiter(registry, 60*time.Second); err != nil {
				return fmt.Errorf("registry not ready: %w", err)
			}
		}

		// Check gateway/agent status (warning only)
		w.checkServicesStatus(req.Context.ComposeProject, req.Mode)
	}

	stagingDir, err := staging.ConfigDir(req.TemplatePath, req.Context.ComposeProject, req.Env)
	if err != nil {
		return err
	}
	var preSnapshot domaincfg.Snapshot
	if w.UserInterface != nil {
		snapshot, err := loadConfigSnapshot(stagingDir)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read existing config: %v", err))
		} else {
			preSnapshot = snapshot
		}
	}

	buildRequest := build.BuildRequest{
		ProjectDir:   req.Context.ProjectDir,
		ProjectName:  req.Context.ComposeProject,
		TemplatePath: req.TemplatePath,
		Env:          req.Env,
		Mode:         req.Mode,
		OutputDir:    req.OutputDir,
		Parameters:   req.Parameters,
		Tag:          req.Tag,
		NoCache:      req.NoCache,
		Verbose:      req.Verbose,
		Bundle:       req.BundleManifest,
		Emoji:        req.Emoji,
	}

	if err := w.Build(buildRequest); err != nil {
		return err
	}

	if w.UserInterface != nil {
		templateConfigDir, err := resolveTemplateConfigDir(req.TemplatePath, req.OutputDir, req.Env)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to resolve template config dir: %v", err))
		} else {
			templateSnapshot, err := loadConfigSnapshot(templateConfigDir)
			if err != nil {
				w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read template config: %v", err))
			} else {
				diff := diffConfigSnapshots(preSnapshot, templateSnapshot)
				emitTemplateDeltaSummary(w.UserInterface, templateConfigDir, diff)
			}
		}

		snapshot, err := loadConfigSnapshot(stagingDir)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read merged config: %v", err))
		} else {
			diff := diffConfigSnapshots(preSnapshot, snapshot)
			emitConfigMergeSummary(w.UserInterface, stagingDir, diff)
		}
	}

	if !req.BuildOnly {
		manifestPath := filepath.Join(stagingDir, "image-import.json")
		manifest, exists, err := loadImageImportManifest(manifestPath)
		if err != nil {
			return err
		}
		if exists && len(manifest.Images) > 0 {
			if imagePrewarm != "all" {
				return fmt.Errorf("image prewarm is required for templates with image functions (use --image-prewarm=all)")
			}
		}
		if imagePrewarm == "all" {
			if err := runImagePrewarm(
				context.Background(),
				w.ComposeRunner,
				w.UserInterface,
				manifestPath,
				req.Verbose,
			); err != nil {
				return err
			}
		}
		if err := w.syncRuntimeConfig(req); err != nil {
			return err
		}

		// Run provisioner
		if err := w.runProvisioner(
			req.Context.ComposeProject,
			req.Mode,
			req.NoDeps,
			req.Verbose,
			req.Context.ProjectDir,
			req.ComposeFiles,
		); err != nil {
			return fmt.Errorf("provisioner failed: %w", err)
		}
	}

	// For containerd mode, function images are pulled by agent/runtime-node.
	// This ensures proper image store management in containerd environments.
	// See: agent/runtime-node IMAGE_PULL_POLICY configuration.

	if w.UserInterface != nil {
		if req.BuildOnly {
			w.UserInterface.Success("Build complete")
		} else {
			w.UserInterface.Success("Deploy complete")
		}
	}
	return nil
}

func normalizeImagePrewarmMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "all", nil
	}
	switch normalized {
	case "off", "all":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid image prewarm mode %q (use off|all)", value)
	}
}

// resolveRegistryAddress resolves the host-side registry address for checks from the host machine.
func (w Workflow) resolveRegistryAddress() string {
	// Check HOST_REGISTRY_ADDR first (host-side registry address)
	hostRegistry := os.Getenv("HOST_REGISTRY_ADDR")
	if hostRegistry != "" {
		return strings.TrimPrefix(hostRegistry, "http://")
	}

	// Use PORT_REGISTRY if set, otherwise default to 5010
	port := os.Getenv("PORT_REGISTRY")
	if port == "" {
		port = "5010"
	}

	return fmt.Sprintf("127.0.0.1:%s", port)
}
