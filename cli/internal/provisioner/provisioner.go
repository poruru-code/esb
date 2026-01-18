// Where: cli/internal/provisioner/provisioner.go
// What: Provisioner entrypoint for DynamoDB/S3 resources.
// Why: Replace Python provisioner with Go-native implementation.
package provisioner

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

const (
	defaultDynamoPort = 8001
	defaultS3Port     = 9000
)

type Runner struct {
	Out          io.Writer
	Clients      ClientFactory
	PortResolver PortResolver
}

func New(client compose.DockerClient) *Runner {
	return &Runner{
		Out:          os.Stdout,
		Clients:      awsClientFactory{},
		PortResolver: dockerPortResolver{Client: client},
	}
}

// Apply implements app.Provisioner interface.
// It applies the desired state defined in ResourcesSpec to the local environment.
func (r *Runner) Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error {
	if r == nil {
		return fmt.Errorf("provisioner is nil")
	}

	out := r.Out
	if out == nil {
		out = os.Stdout
	}
	clients := r.Clients
	if clients == nil {
		return fmt.Errorf("client factory not configured")
	}

	if len(resources.DynamoDB) == 0 && len(resources.S3) == 0 {
		return nil
	}

	if composeProject == "" {
		composeProject = "esb"
	}

	if len(resources.DynamoDB) > 0 {
		port, ok := resolvePort(
			ctx,
			constants.EnvPortDatabase,
			defaultDynamoPort,
			PortRequest{Project: composeProject, Service: "database", ContainerPort: 8000},
			r.PortResolver,
		)
		if !ok {
			fmt.Fprintln(out, "skipping DynamoDB provisioning: port not resolved")
		} else {
			endpoint := fmt.Sprintf("http://localhost:%d", port)
			client, err := clients.DynamoDB(ctx, endpoint)
			if err != nil {
				fmt.Fprintf(out, "skipping DynamoDB provisioning: %v\n", err)
			} else {
				provisionDynamo(ctx, client, resources.DynamoDB, out)
			}
		}
	}

	if len(resources.S3) > 0 {
		port, ok := resolvePort(
			ctx,
			constants.EnvPortS3,
			defaultS3Port,
			PortRequest{Project: composeProject, Service: "s3-storage", ContainerPort: 9000},
			r.PortResolver,
		)
		if !ok {
			fmt.Fprintln(out, "skipping S3 provisioning: port not resolved")
		} else {
			endpoint := fmt.Sprintf("http://localhost:%d", port)
			client, err := clients.S3(ctx, endpoint)
			if err != nil {
				fmt.Fprintf(out, "skipping S3 provisioning: %v\n", err)
			} else {
				provisionS3(ctx, client, resources.S3, out)
			}
		}
	}

	return nil
}
