// Where: cli/internal/infra/deploy/compose_provisioner.go
// What: Compose-based provisioner execution service.
// Why: Keep deploy usecase free from compose operational details.
package deploy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	deployport "github.com/poruru/edge-serverless-box/cli/internal/domain/deployport"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"github.com/poruru/edge-serverless-box/pkg/artifactcore/composeprovision"
)

// ProvisionerRequest captures compose/provisioner execution inputs.
type ProvisionerRequest struct {
	ComposeProject string
	Mode           string
	NoDeps         bool
	Verbose        bool
	ProjectDir     string
	ComposeFiles   []string
}

// ComposeProvisioner provides compose-related operational behavior for deploy.
type ComposeProvisioner = deployport.ComposeProvisioner

type composeProvisioner struct {
	composeRunner compose.CommandRunner
	userInterface ui.UserInterface
	newDocker     dockerClientFactory
}

// NewComposeProvisioner constructs a compose provisioner service.
func NewComposeProvisioner(
	composeRunner compose.CommandRunner,
	userInterface ui.UserInterface,
) ComposeProvisioner {
	return newComposeProvisioner(composeRunner, userInterface, compose.NewDockerClient)
}

func newComposeProvisioner(
	composeRunner compose.CommandRunner,
	userInterface ui.UserInterface,
	factory dockerClientFactory,
) composeProvisioner {
	return composeProvisioner{
		composeRunner: composeRunner,
		userInterface: userInterface,
		newDocker:     factory,
	}
}

// RunProvisioner runs the deploy profile provisioner via docker compose.
func (p composeProvisioner) RunProvisioner(
	composeProject string,
	mode string,
	noDeps bool,
	verbose bool,
	projectDir string,
	composeFiles []string,
) error {
	request := ProvisionerRequest{
		ComposeProject: composeProject,
		Mode:           mode,
		NoDeps:         noDeps,
		Verbose:        verbose,
		ProjectDir:     projectDir,
		ComposeFiles:   composeFiles,
	}
	if p.composeRunner == nil {
		return fmt.Errorf("compose runner is not configured")
	}

	repoRoot, err := config.ResolveRepoRoot(request.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	ctx := context.Background()
	var files []string
	if len(request.ComposeFiles) > 0 {
		files = make([]string, 0, len(request.ComposeFiles))
		for _, file := range request.ComposeFiles {
			normalized := strings.TrimSpace(file)
			if normalized == "" {
				continue
			}
			if !filepath.IsAbs(normalized) {
				normalized = filepath.Join(repoRoot, normalized)
			}
			files = append(files, normalized)
		}
		if len(files) == 0 {
			return fmt.Errorf("compose file is required")
		}
	} else {
		result, err := p.resolveComposeFilesForProject(ctx, request.ComposeProject)
		if err != nil && p.userInterface != nil {
			p.userInterface.Warn(fmt.Sprintf("Warning: failed to resolve compose config files: %v", err))
		}
		if result.SetCount > 1 && p.userInterface != nil {
			p.userInterface.Warn("Warning: multiple compose config sets detected; using the most common one.")
		}
		if len(result.Missing) > 0 && p.userInterface != nil {
			p.userInterface.Warn(
				fmt.Sprintf("Warning: compose config files not found: %s", strings.Join(result.Missing, ", ")),
			)
		}
		files = result.Files
		if len(files) == 0 {
			files = defaultComposeFiles(repoRoot, request.Mode)
		}
	}

	return composeprovision.Execute(ctx, p.composeRunner, repoRoot, composeprovision.Request{
		ComposeProject: request.ComposeProject,
		ComposeFiles:   files,
		NoDeps:         request.NoDeps,
		Verbose:        request.Verbose,
		NoWarnOrphans:  p.composeSupportsNoWarnOrphans(repoRoot),
	})
}
