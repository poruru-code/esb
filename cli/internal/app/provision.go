// Where: cli/internal/app/provision.go
// What: Provisioner interface for up command.
// Why: Allow up to trigger resource provisioning.
package app

import (
	"context"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

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
// Implementations configure the Lambda runtime based on parsed resources.
type Provisioner interface {
	Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error
}
