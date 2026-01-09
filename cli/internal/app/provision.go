// Where: cli/internal/app/provision.go
// What: Provisioner interface for up command.
// Why: Allow up to trigger resource provisioning.
package app

type ProvisionRequest struct {
	TemplatePath   string
	ProjectDir     string
	Env            string
	ComposeProject string
	Mode           string
}

type Provisioner interface {
	Provision(request ProvisionRequest) error
}
