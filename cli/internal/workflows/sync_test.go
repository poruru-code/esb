// Where: cli/internal/workflows/sync_test.go
// What: Unit tests for SyncWorkflow orchestration.
// Why: Validate port discovery and provisioning sequences.
package workflows

import (
	"errors"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestSyncWorkflowRunSuccess(t *testing.T) {
	ctx := state.Context{
		ProjectDir:     "/repo",
		ComposeProject: "esb-dev",
	}
	ui := &testUI{}
	publisher := &recordPortPublisher{
		result: ports.PortPublishResult{
			Published: map[string]int{constants.EnvPortGatewayHTTPS: 1234},
			Changed:   true,
		},
	}
	loader := &recordTemplateLoader{content: "template"}
	parser := &recordTemplateParser{result: generator.ParseResult{Resources: manifest.ResourcesSpec{}}}
	provisioner := &recordProvisioner{}

	workflow := NewSyncWorkflow(
		publisher,
		loader,
		parser,
		provisioner,
		ui,
	)

	req := SyncRequest{
		Context:      ctx,
		Env:          "dev",
		TemplatePath: "/repo/template.yaml",
		Wait:         true,
	}
	result, err := workflow.Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(publisher.calls) != 1 {
		t.Fatalf("expected port publisher to be called")
	}
	if len(loader.paths) != 1 || loader.paths[0] != req.TemplatePath {
		t.Fatalf("expected template loader to read template path")
	}
	if len(parser.contents) != 1 || parser.contents[0] != loader.content {
		t.Fatalf("expected parser to consume template content")
	}
	if len(provisioner.calls) != 1 || provisioner.calls[0].project != ctx.ComposeProject {
		t.Fatalf("expected provisioner to be called")
	}
	if result.Ports.Published[constants.EnvPortGatewayHTTPS] != 1234 {
		t.Fatalf("expected ports result")
	}

	// Verify UI feedback
	foundSyncMsg := false
	for _, msg := range ui.successes {
		if strings.Contains(msg, "Sync complete") {
			foundSyncMsg = true
		}
	}
	if !foundSyncMsg {
		t.Errorf("expected Sync complete success message")
	}
}

func TestSyncWorkflowPublisherError(t *testing.T) {
	ui := &testUI{}
	publisher := &recordPortPublisher{err: errors.New("boom")}
	loader := &recordTemplateLoader{content: "template"}
	parser := &recordTemplateParser{}
	provisioner := &recordProvisioner{}

	workflow := NewSyncWorkflow(publisher, loader, parser, provisioner, ui)
	_, err := workflow.Run(SyncRequest{})
	if err != nil {
		t.Fatalf("Workflow should continue on publisher error, got: %v", err)
	}

	if len(ui.warns) != 1 || !strings.Contains(ui.warns[0], "boom") {
		t.Errorf("expected warning for publisher error")
	}
}

func TestSyncWorkflowProvisionerError(t *testing.T) {
	publisher := &recordPortPublisher{}
	loader := &recordTemplateLoader{content: "template"}
	parser := &recordTemplateParser{}
	provisioner := &recordProvisioner{err: errors.New("failed to provision")}

	workflow := NewSyncWorkflow(publisher, loader, parser, provisioner, nil)
	_, err := workflow.Run(SyncRequest{})
	if err == nil || !strings.Contains(err.Error(), "failed to provision") {
		t.Fatalf("expected provisioner error, got: %v", err)
	}
}
