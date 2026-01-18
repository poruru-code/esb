package app

import (
	"context"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

// Provisioner defines the interface for provisioning Lambda functions and resources.
// Implementations apply the desired state (manifest) to the environment.
type Provisioner interface {
	Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error
}
