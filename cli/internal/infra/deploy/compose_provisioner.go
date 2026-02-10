// Where: cli/internal/infra/deploy/compose_provisioner.go
// What: Compose-based provisioner execution service.
// Why: Keep deploy usecase free from compose operational details.
package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
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
type ComposeProvisioner interface {
	CheckServicesStatus(composeProject, mode string)
	RunProvisioner(request ProvisionerRequest) error
}

type composeProvisioner struct {
	composeRunner compose.CommandRunner
	userInterface ui.UserInterface
}

// NewComposeProvisioner constructs a compose provisioner service.
func NewComposeProvisioner(
	composeRunner compose.CommandRunner,
	userInterface ui.UserInterface,
) ComposeProvisioner {
	return composeProvisioner{
		composeRunner: composeRunner,
		userInterface: userInterface,
	}
}

// RunProvisioner runs the deploy profile provisioner via docker compose.
func (p composeProvisioner) RunProvisioner(request ProvisionerRequest) error {
	if p.composeRunner == nil {
		return fmt.Errorf("compose runner is not configured")
	}

	repoRoot, err := config.ResolveRepoRoot(request.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	ctx := context.Background()
	required := []string{"provisioner", "database", "s3-storage", "victorialogs"}
	var files []string
	if len(request.ComposeFiles) > 0 {
		existing, missing := filterExistingComposeFiles(repoRoot, request.ComposeFiles)
		if len(missing) > 0 || len(existing) == 0 {
			return fmt.Errorf("compose override files not found: %s", strings.Join(missing, ", "))
		}
		files = existing
		ok, missingServices := p.composeHasServices(repoRoot, request.ComposeProject, files, required)
		if !ok {
			return fmt.Errorf("compose override missing services: %s", strings.Join(missingServices, ", "))
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
		if len(files) > 0 {
			ok, missingServices := p.composeHasServices(repoRoot, request.ComposeProject, files, required)
			if !ok {
				return fmt.Errorf("compose config missing services: %s", strings.Join(missingServices, ", "))
			}
		}
		if len(files) == 0 {
			files = defaultComposeFiles(repoRoot, request.Mode)
		}
		if len(files) > 0 {
			ok, missingServices := p.composeHasServices(repoRoot, request.ComposeProject, files, required)
			if !ok {
				return fmt.Errorf("compose config missing services: %s", strings.Join(missingServices, ", "))
			}
		}
	}

	args := []string{"compose"}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	if p.composeSupportsNoWarnOrphans(repoRoot) {
		args = append(args, "--no-warn-orphans")
	}
	args = append(args, "--profile", "deploy")
	if request.ComposeProject != "" {
		args = append(args, "-p", request.ComposeProject)
	}
	args = append(args, "run", "--rm")
	if request.NoDeps {
		args = append(args, "--no-deps")
	}
	args = append(args, "provisioner")
	if request.Verbose {
		if err := p.composeRunner.Run(ctx, repoRoot, "docker", args...); err != nil {
			return fmt.Errorf("run provisioner: %w", err)
		}
		return nil
	}
	if err := p.composeRunner.RunQuiet(ctx, repoRoot, "docker", args...); err != nil {
		return fmt.Errorf("run provisioner: %w", err)
	}
	return nil
}
