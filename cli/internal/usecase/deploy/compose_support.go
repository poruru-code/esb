// Where: cli/internal/usecase/deploy/compose_support.go
// What: Thin wrappers around infra compose provisioner service.
// Why: Keep usecase focused on orchestration, not compose command details.
package deploy

import (
	infradeploy "github.com/poruru/edge-serverless-box/cli/internal/infra/deploy"
)

func (w Workflow) checkServicesStatus(composeProject, mode string) {
	provisioner := w.composeProvisioner()
	if provisioner == nil {
		return
	}
	provisioner.CheckServicesStatus(composeProject, mode)
}

func (w Workflow) runProvisioner(
	composeProject,
	mode string,
	noDeps bool,
	verbose bool,
	projectDir string,
	composeFiles []string,
) error {
	provisioner := w.composeProvisioner()
	if provisioner == nil {
		return nil
	}
	return provisioner.RunProvisioner(infradeploy.ProvisionerRequest{
		ComposeProject: composeProject,
		Mode:           mode,
		NoDeps:         noDeps,
		Verbose:        verbose,
		ProjectDir:     projectDir,
		ComposeFiles:   composeFiles,
	})
}

func (w Workflow) composeProvisioner() infradeploy.ComposeProvisioner {
	if w.ComposeProvisioner != nil {
		return w.ComposeProvisioner
	}
	return infradeploy.NewComposeProvisioner(w.ComposeRunner, w.UserInterface)
}
