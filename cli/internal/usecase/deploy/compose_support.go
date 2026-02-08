// Where: cli/internal/usecase/deploy/compose_support.go
// What: Compose/service checks and provisioner execution helpers.
// Why: Separate compose-specific operational logic from deploy orchestration flow.
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
)

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
func (w Workflow) runProvisioner(
	composeProject,
	mode string,
	noDeps bool,
	verbose bool,
	projectDir string,
	composeFiles []string,
) error {
	repoRoot, err := config.ResolveRepoRoot(projectDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	ctx := context.Background()
	required := []string{"provisioner", "database", "s3-storage", "victorialogs"}
	var files []string
	if len(composeFiles) > 0 {
		existing, missing := filterExistingComposeFiles(repoRoot, composeFiles)
		if len(missing) > 0 || len(existing) == 0 {
			return fmt.Errorf("compose override files not found: %s", strings.Join(missing, ", "))
		}
		files = existing
		if w.ComposeRunner != nil {
			ok, missingServices := w.composeHasServices(repoRoot, composeProject, files, required)
			if !ok {
				return fmt.Errorf("compose override missing services: %s", strings.Join(missingServices, ", "))
			}
		}
	} else {
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
		files = result.Files
		if len(files) > 0 && w.ComposeRunner != nil {
			ok, missingServices := w.composeHasServices(repoRoot, composeProject, files, required)
			if !ok {
				return fmt.Errorf("compose config missing services: %s", strings.Join(missingServices, ", "))
			}
		}
		if len(files) == 0 {
			files = defaultComposeFiles(repoRoot, mode)
		}
		if len(files) > 0 && w.ComposeRunner != nil {
			ok, missingServices := w.composeHasServices(repoRoot, composeProject, files, required)
			if !ok {
				return fmt.Errorf("compose config missing services: %s", strings.Join(missingServices, ", "))
			}
		}
	}

	args := []string{"compose"}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	if w.composeSupportsNoWarnOrphans(repoRoot) {
		args = append(args, "--no-warn-orphans")
	}
	args = append(args, "--profile", "deploy")
	if composeProject != "" {
		args = append(args, "-p", composeProject)
	}
	args = append(args, "run", "--rm")
	if noDeps {
		args = append(args, "--no-deps")
	}
	args = append(args, "provisioner")
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

func filterExistingComposeFiles(repoRoot string, files []string) ([]string, []string) {
	existing := make([]string, 0, len(files))
	missing := []string{}
	for _, file := range files {
		path := strings.TrimSpace(file)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && strings.TrimSpace(repoRoot) != "" {
			path = filepath.Join(repoRoot, path)
		}
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, path)
			continue
		}
		existing = append(existing, path)
	}
	return existing, missing
}

func defaultComposeFiles(repoRoot, mode string) []string {
	files := []string{}
	if mode == compose.ModeContainerd {
		files = append(files, filepath.Join(repoRoot, "docker-compose.containerd.yml"))
	} else {
		files = append(files, filepath.Join(repoRoot, "docker-compose.docker.yml"))
	}
	if proxyFile := resolveProxyComposeFile(repoRoot, mode); proxyFile != "" {
		files = append(files, proxyFile)
	}
	return files
}

func (w Workflow) composeHasServices(
	repoRoot string,
	composeProject string,
	files []string,
	required []string,
) (bool, []string) {
	if w.ComposeRunner == nil || len(files) == 0 {
		return true, nil
	}
	ctx := context.Background()
	args := []string{"compose"}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	if strings.TrimSpace(composeProject) != "" {
		args = append(args, "-p", composeProject)
	}
	args = append(args, "--profile", "deploy", "config", "--services")
	out, err := w.ComposeRunner.RunOutput(ctx, repoRoot, "docker", args...)
	if err != nil {
		return false, required
	}
	services := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		services[trimmed] = struct{}{}
	}
	missing := []string{}
	for _, name := range required {
		if _, ok := services[name]; !ok {
			missing = append(missing, name)
		}
	}
	return len(missing) == 0, missing
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
