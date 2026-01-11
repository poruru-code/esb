// Where: cli/internal/app/provision.go
// What: Provisioner interface for up command.
// Why: Allow up to trigger resource provisioning.
package app

// ProvisionRequest contains parameters for provisioning Lambda functions.
// It specifies template location, project setup, and runtime mode.
type ProvisionRequest struct {
	TemplatePath   string
	ProjectDir     string
	Env            string
	ComposeProject string
	Mode           string
}

// Provisioner defines the interface for provisioning Lambda functions.
// Implementations parse the SAM template and configure the Lambda runtime.
type Provisioner interface {
	Provision(request ProvisionRequest) error
}
