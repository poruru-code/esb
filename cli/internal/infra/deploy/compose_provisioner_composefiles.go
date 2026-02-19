// Where: cli/internal/infra/deploy/compose_provisioner_composefiles.go
// What: Compose file resolution helpers.
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

type dockerClientFactory func() (compose.DockerClient, error)

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

func (p composeProvisioner) resolveComposeFilesForProject(
	ctx context.Context,
	composeProject string,
) (compose.FilesResult, error) {
	trimmedProject := strings.TrimSpace(composeProject)
	if trimmedProject == "" {
		return compose.FilesResult{}, nil
	}
	if p.newDocker == nil {
		return compose.FilesResult{}, fmt.Errorf("docker client factory is not configured")
	}
	client, err := p.newDocker()
	if err != nil {
		return compose.FilesResult{}, fmt.Errorf("create docker client: %w", err)
	}
	if client == nil {
		return compose.FilesResult{}, fmt.Errorf("docker client factory returned nil client")
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
