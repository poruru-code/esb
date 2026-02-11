// Where: cli/internal/usecase/deploy/compose_support.go
// What: Thin wrappers around infra compose provisioner service.
// Why: Keep usecase focused on orchestration, not compose command details.
package deploy

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
	return provisioner.RunProvisioner(
		composeProject,
		mode,
		noDeps,
		verbose,
		projectDir,
		composeFiles,
	)
}

func (w Workflow) composeProvisioner() ComposeProvisioner {
	return w.ComposeProvisioner
}
