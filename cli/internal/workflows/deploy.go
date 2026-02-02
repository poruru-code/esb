// Where: cli/internal/workflows/deploy.go
// What: Deploy workflow orchestration.
// Why: Encapsulate deploy-specific logic without CLI concerns.
package workflows

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// DeployRequest captures the inputs required to run a deploy.
type DeployRequest struct {
	Context      state.Context
	Env          string
	TemplatePath string
	Mode         string
	OutputDir    string
	Parameters   map[string]string
	Tag          string
	NoCache      bool
	Verbose      bool
}

// RegistryChecker defines the interface for checking registry readiness.
type RegistryChecker interface {
	WaitReady(registry string, timeout time.Duration) error
}

// defaultRegistryChecker implements RegistryChecker using HTTP checks.
type defaultRegistryChecker struct{}

func (c defaultRegistryChecker) WaitReady(registry string, timeout time.Duration) error {
	if registry == "" {
		return nil
	}

	url := fmt.Sprintf("http://%s/v2/", registry)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			// Only accept 200 OK as success (not 401/404/etc)
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("registry not responding at %s", url)
}

// DeployWorkflow executes the deploy orchestration steps.
type DeployWorkflow struct {
	Builder         ports.Builder
	EnvApplier      ports.RuntimeEnvApplier
	UserInterface   ports.UserInterface
	ComposeRunner   compose.CommandRunner
	RegistryChecker RegistryChecker
}

// NewDeployWorkflow constructs a DeployWorkflow.
func NewDeployWorkflow(
	builder ports.Builder,
	envApplier ports.RuntimeEnvApplier,
	ui ports.UserInterface,
	composeRunner compose.CommandRunner,
) DeployWorkflow {
	return DeployWorkflow{
		Builder:         builder,
		EnvApplier:      envApplier,
		UserInterface:   ui,
		ComposeRunner:   composeRunner,
		RegistryChecker: defaultRegistryChecker{},
	}
}

// Run executes the deploy workflow.
func (w DeployWorkflow) Run(req DeployRequest) error {
	if w.Builder == nil {
		return fmt.Errorf("builder port is not configured")
	}
	if w.ComposeRunner == nil {
		return fmt.Errorf("compose runner is not configured")
	}
	if w.EnvApplier != nil {
		if err := w.EnvApplier.Apply(req.Context); err != nil {
			return err
		}
	}

	// Wait for registry to be ready
	registry := w.resolveRegistryAddress()
	if w.RegistryChecker != nil {
		if err := w.RegistryChecker.WaitReady(registry, 60*time.Second); err != nil {
			return fmt.Errorf("registry not ready: %w", err)
		}
	}

	// Check gateway/agent status (warning only)
	w.checkServicesStatus(req.Context.ComposeProject, req.Mode)

	buildRequest := generator.BuildRequest{
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
		Bundle:       false,
	}

	if err := w.Builder.Build(buildRequest); err != nil {
		return err
	}

	// Run provisioner
	if err := w.runProvisioner(req.Context.ComposeProject, req.Mode, req.Verbose, req.Context.ProjectDir); err != nil {
		return fmt.Errorf("provisioner failed: %w", err)
	}

	// For containerd mode, function images are pulled by agent/runtime-node.
	// This ensures proper image store management in containerd environments.
	// See: agent/runtime-node IMAGE_PULL_POLICY configuration.

	if w.UserInterface != nil {
		w.UserInterface.Success("✓ Deploy complete")
	}
	return nil
}

// resolveRegistryAddress resolves the host-side registry address for checks from the host machine.
func (w DeployWorkflow) resolveRegistryAddress() string {
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

// checkServicesStatus checks if gateway and agent are running (warning only).
func (w DeployWorkflow) checkServicesStatus(composeProject, mode string) {
	if w.UserInterface == nil {
		return
	}

	// Check gateway
	gatewayRunning := w.isServiceRunning(composeProject, "gateway")
	if !gatewayRunning {
		w.UserInterface.Warn("Warning: Gateway is not running. Deploy will continue but functions may not be immediately available.")
	}

	// Check agent (docker mode) or runtime-node (containerd mode)
	agentService := "agent"
	if mode == compose.ModeContainerd {
		agentService = "runtime-node"
	}
	agentRunning := w.isServiceRunning(composeProject, agentService)
	if !agentRunning {
		w.UserInterface.Warn(fmt.Sprintf("Warning: %s is not running. Deploy will continue but function execution may fail.", agentService))
	}
}

// isServiceRunning checks if a compose service is running.
func (w DeployWorkflow) isServiceRunning(composeProject, service string) bool {
	if w.ComposeRunner == nil {
		return true // Skip check if no runner available
	}
	ctx := context.Background()
	// Use docker compose ps to check if service is running
	// -q returns container ID if running, empty if stopped/not found
	out, err := w.ComposeRunner.RunOutput(ctx, "", "docker", "compose", "-p", composeProject, "ps", "-q", service)
	if err != nil {
		return false
	}
	// Empty output means service is not running
	return len(out) > 0
}

// runProvisioner runs the provisioner using docker compose.
func (w DeployWorkflow) runProvisioner(composeProject, mode string, verbose bool, projectDir string) error {
	repoRoot, err := config.ResolveRepoRoot(projectDir)
	if err != nil {
		return err
	}

	// Determine compose file based on mode
	var composeFile string
	if mode == compose.ModeContainerd {
		composeFile = filepath.Join(repoRoot, "docker-compose.containerd.yml")
	} else {
		composeFile = filepath.Join(repoRoot, "docker-compose.docker.yml")
	}

	args := []string{"compose", "-f", composeFile, "--profile", "deploy"}
	if composeProject != "" {
		args = append(args, "-p", composeProject)
	}
	args = append(args, "run", "--rm", "provisioner")

	ctx := context.Background()
	if verbose {
		return w.ComposeRunner.Run(ctx, repoRoot, "docker", args...)
	}
	return w.ComposeRunner.RunQuiet(ctx, repoRoot, "docker", args...)
}

// DeployDeps holds deploy-specific dependencies.
type DeployDeps struct {
	Builder       ports.Builder
	ComposeRunner compose.CommandRunner
}
