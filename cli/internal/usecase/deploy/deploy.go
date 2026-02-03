// Where: cli/internal/usecase/deploy/deploy.go
// What: Deploy workflow orchestration.
// Why: Encapsulate deploy-specific logic without CLI concerns.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
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
	Build           func(generator.BuildRequest) error
	ApplyRuntimeEnv func(state.Context) error
	UserInterface   ui.UserInterface
	ComposeRunner   compose.CommandRunner
	RegistryWaiter  RegistryWaiter
}

// NewDeployWorkflow constructs a Workflow.
func NewDeployWorkflow(
	build func(generator.BuildRequest) error,
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

	req = w.alignGatewayRuntime(req)

	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	// Wait for registry to be ready
	registry := w.resolveRegistryAddress()
	if w.RegistryWaiter != nil {
		if err := w.RegistryWaiter(registry, 60*time.Second); err != nil {
			return fmt.Errorf("registry not ready: %w", err)
		}
	}

	// Check gateway/agent status (warning only)
	w.checkServicesStatus(req.Context.ComposeProject, req.Mode)

	stagingDir := staging.ConfigDir(req.Context.ComposeProject, req.Env)
	var preSnapshot domaincfg.Snapshot
	if w.UserInterface != nil {
		snapshot, err := loadConfigSnapshot(stagingDir)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read existing config: %v", err))
		} else {
			preSnapshot = snapshot
		}
	}

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

	if err := w.syncRuntimeConfig(req); err != nil {
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

type gatewayRuntimeInfo struct {
	ComposeProject    string
	ProjectName       string
	ContainersNetwork string
}

func (w Workflow) alignGatewayRuntime(req Request) Request {
	if skipGatewayAlign() {
		return req
	}
	info, err := resolveGatewayRuntime(req.Context.ComposeProject)
	if err != nil {
		if w.UserInterface != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to resolve gateway runtime: %v", err))
		}
		return req
	}
	if info.ComposeProject != "" && info.ComposeProject != req.Context.ComposeProject {
		if w.UserInterface != nil {
			w.UserInterface.Warn(
				fmt.Sprintf("Warning: using running gateway project %q (was %q)", info.ComposeProject, req.Context.ComposeProject),
			)
		}
		req.Context.ComposeProject = info.ComposeProject
	}
	if info.ProjectName != "" && strings.TrimSpace(os.Getenv(constants.EnvProjectName)) != info.ProjectName {
		_ = os.Setenv(constants.EnvProjectName, info.ProjectName)
	}
	if info.ContainersNetwork != "" && strings.TrimSpace(os.Getenv(constants.EnvNetworkExternal)) != info.ContainersNetwork {
		_ = os.Setenv(constants.EnvNetworkExternal, info.ContainersNetwork)
	}
	w.warnInfraNetworkMismatch(req.Context.ComposeProject, info.ContainersNetwork)
	return req
}

func skipGatewayAlign() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ESB_SKIP_GATEWAY_ALIGN")))
	return value == "1" || value == "true" || value == "yes"
}

func resolveGatewayRuntime(composeProject string) (gatewayRuntimeInfo, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("create docker client: %w", err)
	}
	rawClient, ok := client.(*dockerclient.Client)
	if !ok {
		return gatewayRuntimeInfo{}, errUnsupportedDockerClient
	}

	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
	if strings.TrimSpace(composeProject) != "" {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	}
	containers, err := rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("list containers: %w", err)
	}
	if len(containers) == 0 && strings.TrimSpace(composeProject) != "" {
		filterArgs = filters.NewArgs()
		filterArgs.Add("label", fmt.Sprintf("%s=gateway", compose.ComposeServiceLabel))
		containers, err = rawClient.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
		if err != nil {
			return gatewayRuntimeInfo{}, fmt.Errorf("list containers: %w", err)
		}
	}
	if len(containers) == 0 {
		return gatewayRuntimeInfo{}, nil
	}

	selected := containers[0]
	for _, ctr := range containers {
		if strings.EqualFold(ctr.State, "running") {
			selected = ctr
			break
		}
	}
	inspect, err := rawClient.ContainerInspect(ctx, selected.ID)
	if err != nil {
		return gatewayRuntimeInfo{}, fmt.Errorf("inspect container: %w", err)
	}
	envMap := envSliceToMap(inspect.Config.Env)
	info := gatewayRuntimeInfo{
		ComposeProject:    strings.TrimSpace(selected.Labels[compose.ComposeProjectLabel]),
		ProjectName:       strings.TrimSpace(envMap[constants.EnvProjectName]),
		ContainersNetwork: strings.TrimSpace(envMap["CONTAINERS_NETWORK"]),
	}
	if info.ContainersNetwork == "" {
		info.ContainersNetwork = pickGatewayNetwork(inspect.NetworkSettings.Networks)
	}
	return info, nil
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		out[key] = value
	}
	return out
}

func pickGatewayNetwork(networks map[string]*network.EndpointSettings) string {
	for name := range networks {
		if strings.Contains(name, "external") {
			return name
		}
	}
	for name := range networks {
		return name
	}
	return ""
}

func (w Workflow) warnInfraNetworkMismatch(composeProject, gatewayNetwork string) {
	if w.UserInterface == nil {
		return
	}
	if strings.TrimSpace(gatewayNetwork) == "" {
		return
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	if strings.TrimSpace(composeProject) != "" {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	}
	containers, err := client.ContainerList(ctx, container.ListOptions{All: false, Filters: filterArgs})
	if err != nil {
		return
	}
	required := map[string]struct{}{
		"database":     {},
		"s3-storage":   {},
		"victorialogs": {},
	}
	missing := make([]string, 0, len(required))
	for _, ctr := range containers {
		service := strings.TrimSpace(ctr.Labels[compose.ComposeServiceLabel])
		if _, ok := required[service]; !ok {
			continue
		}
		if !containerOnNetwork(&ctr, gatewayNetwork) {
			missing = append(missing, service)
		}
	}
	if len(missing) == 0 {
		return
	}
	w.UserInterface.Warn(
		fmt.Sprintf(
			"Warning: gateway network %q is missing services: %s. Recreate the stack or attach the services to that network.",
			gatewayNetwork,
			strings.Join(missing, ", "),
		),
	)
}

func containerOnNetwork(ctr *container.Summary, network string) bool {
	if ctr == nil || ctr.NetworkSettings == nil {
		return false
	}
	for name := range ctr.NetworkSettings.Networks {
		if name == network {
			return true
		}
	}
	return false
}

type runtimeConfigTarget struct {
	BindPath    string
	VolumeName  string
	ContainerID string
}

func (w Workflow) syncRuntimeConfig(req Request) error {
	composeProject := strings.TrimSpace(req.Context.ComposeProject)
	if composeProject == "" {
		return nil
	}
	stagingDir := staging.ConfigDir(composeProject, req.Env)
	if _, err := os.Stat(stagingDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staging dir: %w", err)
	}
	target, err := resolveRuntimeConfigTarget(composeProject)
	if err != nil {
		return err
	}
	if target.BindPath == "" && target.VolumeName == "" && target.ContainerID == "" {
		return nil
	}
	if target.BindPath != "" {
		if samePath(target.BindPath, stagingDir) {
			return nil
		}
		return copyConfigFiles(stagingDir, target.BindPath)
	}
	var containerErr error
	if target.ContainerID != "" {
		if err := copyConfigToContainer(w.ComposeRunner, stagingDir, target.ContainerID); err == nil {
			return nil
		}
		containerErr = err
	}
	if target.VolumeName != "" {
		err := copyConfigToVolume(w.ComposeRunner, stagingDir, target.VolumeName)
		if err == nil {
			return nil
		}
		if containerErr != nil {
			return fmt.Errorf("sync runtime config failed: %w", errors.Join(containerErr, err))
		}
		return fmt.Errorf("sync runtime config failed: %w", err)
	}
	return containerErr
}

func resolveRuntimeConfigTarget(composeProject string) (runtimeConfigTarget, error) {
	client, err := compose.NewDockerClient()
	if err != nil {
		return runtimeConfigTarget{}, fmt.Errorf("create docker client: %w", err)
	}
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", compose.ComposeProjectLabel, composeProject))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return runtimeConfigTarget{}, fmt.Errorf("list containers: %w", err)
	}
	var fallback *container.Summary
	for i, ctr := range containers {
		if ctr.Labels == nil {
			continue
		}
		if ctr.Labels[compose.ComposeServiceLabel] == "gateway" {
			fallback = &containers[i]
			break
		}
	}
	if fallback == nil && len(containers) > 0 {
		fallback = &containers[0]
	}
	if fallback == nil {
		return runtimeConfigTarget{}, nil
	}
	target := runtimeConfigTarget{ContainerID: fallback.ID}
	for _, mount := range fallback.Mounts {
		if mount.Destination != "/app/runtime-config" {
			continue
		}
		if strings.EqualFold(string(mount.Type), "bind") {
			target.BindPath = mount.Source
			return target, nil
		}
		if strings.EqualFold(string(mount.Type), "volume") {
			if mount.Name != "" {
				target.VolumeName = mount.Name
				return target, nil
			}
			if mount.Source != "" {
				target.BindPath = mount.Source
				return target, nil
			}
		}
	}
	return target, nil
}

func copyConfigFiles(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml"} {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := filepath.Join(destDir, name)
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyConfigToVolume(runner compose.CommandRunner, srcDir, volume string) error {
	if runner == nil {
		return errComposeRunnerNotConfigured
	}
	cmd := "mkdir -p /app/runtime-config && " +
		"for f in functions.yml routing.yml resources.yml; do " +
		"if [ -f \"/src/${f}\" ]; then cp -f \"/src/${f}\" \"/app/runtime-config/${f}\"; fi; " +
		"done"
	args := []string{
		"run",
		"--rm",
		"-v", volume + ":/app/runtime-config",
		"-v", srcDir + ":/src:ro",
		"alpine",
		"sh",
		"-c",
		cmd,
	}
	if err := runner.Run(context.Background(), "", "docker", args...); err != nil {
		return fmt.Errorf("copy config to volume: %w", err)
	}
	return nil
}

func copyConfigToContainer(runner compose.CommandRunner, srcDir, containerID string) error {
	if runner == nil {
		return errComposeRunnerNotConfigured
	}
	ctx := context.Background()
	for _, name := range []string{"functions.yml", "routing.yml", "resources.yml"} {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := containerID + ":/app/runtime-config/" + name
		if err := runner.Run(ctx, "", "docker", "cp", src, dest); err != nil {
			return fmt.Errorf("copy config to container: %w", err)
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dest, err)
	}
	return nil
}

func samePath(left, right string) bool {
	l, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	r, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return filepath.Clean(l) == filepath.Clean(r)
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

// checkServicesStatus checks if gateway and agent are running (warning only).
func (w Workflow) checkServicesStatus(composeProject, mode string) {
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
func (w Workflow) isServiceRunning(composeProject, service string) bool {
	if w.ComposeRunner == nil {
		return true // Skip check if no runner available
	}
	ctx := context.Background()
	args := []string{"compose"}
	if result, err := w.resolveComposeFilesForProject(ctx, composeProject); err == nil {
		for _, file := range result.Files {
			args = append(args, "-f", file)
		}
	}
	if strings.TrimSpace(composeProject) != "" {
		args = append(args, "-p", composeProject)
	}
	// Use docker compose ps to check if service is running
	// -q returns container ID if running, empty if stopped/not found
	out, err := w.ComposeRunner.RunOutput(ctx, "", "docker", append(args, "ps", "-q", service)...)
	if err != nil {
		return false
	}
	// Empty output means service is not running
	return len(out) > 0
}

// runProvisioner runs the provisioner using docker compose.
func (w Workflow) runProvisioner(composeProject, mode string, verbose bool, projectDir string) error {
	repoRoot, err := config.ResolveRepoRoot(projectDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	ctx := context.Background()
	result, err := w.resolveComposeFilesForProject(ctx, composeProject)
	if err != nil && w.UserInterface != nil {
		w.UserInterface.Warn(fmt.Sprintf("Warning: failed to resolve compose config files: %v", err))
	}
	if result.SetCount > 1 && w.UserInterface != nil {
		w.UserInterface.Warn("Warning: multiple compose config sets detected; using the most common one.")
	}
	if len(result.Missing) > 0 && w.UserInterface != nil {
		w.UserInterface.Warn(fmt.Sprintf("Warning: compose config files not found: %s", strings.Join(result.Missing, ", ")))
	}

	args := []string{"compose"}
	if len(result.Files) > 0 {
		for _, file := range result.Files {
			args = append(args, "-f", file)
		}
	} else {
		// Determine compose file based on mode
		var composeFile string
		if mode == compose.ModeContainerd {
			composeFile = filepath.Join(repoRoot, "docker-compose.containerd.yml")
		} else {
			composeFile = filepath.Join(repoRoot, "docker-compose.docker.yml")
		}
		proxyFile := resolveProxyComposeFile(repoRoot, mode)
		args = append(args, "-f", composeFile)
		if proxyFile != "" {
			args = append(args, "-f", proxyFile)
		}
	}
	if w.composeSupportsNoWarnOrphans(repoRoot) {
		args = append(args, "--no-warn-orphans")
	}
	args = append(args, "--profile", "deploy")
	if composeProject != "" {
		args = append(args, "-p", composeProject)
	}
	args = append(args, "run", "--rm", "provisioner")
	if verbose {
		if err := w.ComposeRunner.Run(ctx, repoRoot, "docker", args...); err != nil {
			return fmt.Errorf("run provisioner: %w", err)
		}
		return nil
	}
	if err := w.ComposeRunner.RunQuiet(ctx, repoRoot, "docker", args...); err != nil {
		return fmt.Errorf("run provisioner: %w", err)
	}
	return nil
}

func (w Workflow) resolveComposeFilesForProject(ctx context.Context, composeProject string) (compose.FilesResult, error) {
	trimmedProject := strings.TrimSpace(composeProject)
	if trimmedProject == "" {
		return compose.FilesResult{}, nil
	}
	client, err := compose.NewDockerClient()
	if err != nil {
		return compose.FilesResult{}, fmt.Errorf("create docker client: %w", err)
	}
	result, err := compose.ResolveComposeFilesFromProject(ctx, client, trimmedProject)
	if err != nil {
		return compose.FilesResult{}, fmt.Errorf("resolve compose files: %w", err)
	}
	return result, nil
}

func (w Workflow) composeSupportsNoWarnOrphans(repoRoot string) bool {
	if w.ComposeRunner == nil {
		return false
	}
	ctx := context.Background()
	out, err := w.ComposeRunner.RunOutput(ctx, repoRoot, "docker", "compose", "--help")
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "--no-warn-orphans")
}

func resolveProxyComposeFile(repoRoot, mode string) string {
	filename := "docker-compose.proxy.docker.yml"
	if mode == compose.ModeContainerd {
		filename = "docker-compose.proxy.containerd.yml"
	}
	path := filepath.Join(repoRoot, filename)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
