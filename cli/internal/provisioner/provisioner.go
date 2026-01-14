// Where: cli/internal/provisioner/provisioner.go
// What: Provisioner entrypoint for DynamoDB/S3 resources.
// Why: Replace Python provisioner with Go-native implementation.
package provisioner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
)

const (
	defaultDynamoPort = 8001
	defaultS3Port     = 9000
)

type Request struct {
	TemplatePath   string
	ProjectDir     string
	Env            string
	ComposeProject string
	Mode           string
}

type Provisioner interface {
	Provision(request Request) error
}

type Runner struct {
	Out          io.Writer
	Clients      ClientFactory
	PortResolver PortResolver
	Parser       generator.Parser
}

func New(client compose.DockerClient) *Runner {
	return &Runner{
		Out:          os.Stdout,
		Clients:      awsClientFactory{},
		PortResolver: dockerPortResolver{Client: client},
		Parser:       generator.DefaultParser{},
	}
}

func (r *Runner) Provision(request Request) error {
	if r == nil {
		return fmt.Errorf("provisioner is nil")
	}
	if strings.TrimSpace(request.TemplatePath) == "" {
		return fmt.Errorf("template path is required")
	}

	out := r.Out
	if out == nil {
		out = os.Stdout
	}
	clients := r.Clients
	if clients == nil {
		return fmt.Errorf("client factory not configured")
	}
	parser := r.Parser
	if parser == nil {
		parser = generator.DefaultParser{}
	}

	templatePath, err := filepath.Abs(request.TemplatePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(templatePath); err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}

	parsed, err := parser.Parse(string(content), nil)
	if err != nil {
		return err
	}

	if len(parsed.Resources.DynamoDB) == 0 && len(parsed.Resources.S3) == 0 {
		return nil
	}

	ctx := context.Background()
	composeProject := normalizeComposeProject(request.ComposeProject, request.Env)

	if len(parsed.Resources.DynamoDB) > 0 {
		port, ok := resolvePort(
			ctx,
			"ESB_PORT_DATABASE",
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
				provisionDynamo(ctx, client, parsed.Resources.DynamoDB, out)
			}
		}
	}

	if len(parsed.Resources.S3) > 0 {
		port, ok := resolvePort(
			ctx,
			"ESB_PORT_S3",
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
				provisionS3(ctx, client, parsed.Resources.S3, out)
			}
		}
	}

	return nil
}

func normalizeComposeProject(explicit, env string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if strings.TrimSpace(env) == "" {
		return "esb"
	}
	return fmt.Sprintf("esb-%s", strings.ToLower(env))
}
