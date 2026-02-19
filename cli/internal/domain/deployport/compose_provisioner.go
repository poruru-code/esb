// Where: cli/internal/domain/deployport/compose_provisioner.go
// What: Cross-layer compose provisioner contract.
// Why: Share a single interface definition across command/usecase/infra without layer leaks.
package deployport

// ComposeProvisioner defines compose-related operational behavior consumed by deploy flows.
type ComposeProvisioner interface {
	CheckServicesStatus(composeProject, mode string)
	RunProvisioner(
		composeProject string,
		mode string,
		noDeps bool,
		verbose bool,
		projectDir string,
		composeFiles []string,
	) error
}
