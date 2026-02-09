// Where: cli/internal/usecase/deploy/deploy.go
// What: Deploy workflow orchestration.
// Why: Encapsulate deploy-specific logic without CLI concerns.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	if strings.TrimSpace(registry) == "" {
		return nil
	}
	probeURLs := resolveRegistryProbeURLs(registry)
	if len(probeURLs) == 0 {
		return fmt.Errorf("invalid registry address: %s", registry)
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		for _, probeURL := range probeURLs {
			client := registryWaitHTTPClient(probeURL)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, probeURL, nil)
			if err != nil {
				return fmt.Errorf("create registry request: %w", err)
			}
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				// 2xx-4xx means endpoint is reachable and the registry is up.
				if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
					return nil
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%w at %s", errRegistryNotResponding, probeURLs[0])
}

func resolveRegistryProbeURLs(registry string) []string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && strings.TrimSpace(parsed.Host) != "" {
			parsed.Path = "/v2/"
			parsed.RawPath = ""
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return []string{parsed.String()}
		}
	}

	host := strings.TrimPrefix(trimmed, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimSuffix(host, "/")
	if slash := strings.Index(host, "/"); slash != -1 {
		host = host[:slash]
	}
	if host == "" {
		return nil
	}

	schemes := []string{"https", "http"}
	if shouldBypassRegistryProxy(resolveRegistryHost(host)) {
		schemes = []string{"http", "https"}
	}

	urls := make([]string, 0, len(schemes))
	for _, scheme := range schemes {
		urls = append(urls, (&url.URL{Scheme: scheme, Host: host, Path: "/v2/"}).String())
	}
	return urls
}

func registryWaitHTTPClient(probeURL string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	parsed, err := url.Parse(probeURL)
	if err == nil && shouldBypassRegistryProxy(parsed.Hostname()) {
		transport.Proxy = nil
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
	}
}

func resolveRegistryHost(registry string) string {
	trimmed := strings.TrimSpace(registry)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.TrimSpace(parsed.Hostname())
		}
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	if slash := strings.Index(trimmed, "/"); slash != -1 {
		trimmed = trimmed[:slash]
	}
	host := strings.TrimSpace(trimmed)
	if host == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		return strings.TrimSpace(splitHost)
	}
	return strings.Trim(host, "[]")
}

func shouldBypassRegistryProxy(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	switch normalized {
	case "localhost", "127.0.0.1", "registry", "host.docker.internal":
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
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
	hostRegistry := strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR"))
	if hostRegistry != "" {
		hostRegistry = strings.TrimPrefix(hostRegistry, "http://")
		hostRegistry = strings.TrimPrefix(hostRegistry, "https://")
		hostRegistry = strings.TrimSuffix(hostRegistry, "/")
		return hostRegistry
	}

	// Use PORT_REGISTRY if set, otherwise default to 5010
	port := os.Getenv("PORT_REGISTRY")
	if port == "" {
		port = "5010"
	}

	return fmt.Sprintf("127.0.0.1:%s", port)
}
