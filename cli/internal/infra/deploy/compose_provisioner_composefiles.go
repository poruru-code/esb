// Where: cli/internal/infra/deploy/compose_provisioner_composefiles.go
// What: Compose file resolution and service validation helpers.
// Why: Isolate compose configuration mechanics from provisioner orchestration.
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

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

func (p composeProvisioner) composeHasServices(
	repoRoot string,
	composeProject string,
	files []string,
	required []string,
) (bool, []string) {
	if p.composeRunner == nil || len(files) == 0 {
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
	out, err := p.composeRunner.RunOutput(ctx, repoRoot, "docker", args...)
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

func (p composeProvisioner) resolveComposeFilesForProject(
	ctx context.Context,
	composeProject string,
) (compose.FilesResult, error) {
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

func (p composeProvisioner) composeSupportsNoWarnOrphans(repoRoot string) bool {
	if p.composeRunner == nil {
		return false
	}
	ctx := context.Background()
	out, err := p.composeRunner.RunOutput(ctx, repoRoot, "docker", "compose", "--help")
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
