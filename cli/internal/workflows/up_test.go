// Where: cli/internal/workflows/up_test.go
// What: Unit tests for UpWorkflow orchestration.
// Why: Validate reset/build/port/provision sequences without CLI adapters.
package workflows

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

func TestUpWorkflowRunSuccess(t *testing.T) {
	ctx := state.Context{
		ProjectDir:     "/repo",
		ComposeProject: "esb-dev",
	}
	ui := &testUI{}
	envApplier := &recordEnvApplier{}
	builder := &recordBuilder{}
	upper := &recordUpper{}
	downer := &recordDowner{}
	publisher := &recordPortPublisher{
		ports: map[string]int{constants.EnvPortGatewayHTTPS: 1234},
	}
	creds := &recordCredentialManager{
		creds: ports.AuthCredentials{
			AuthUser:  "user",
			AuthPass:  "pass",
			Generated: true,
		},
	}
	loader := &recordTemplateLoader{content: "template"}
	parser := &recordTemplateParser{result: generator.ParseResult{Resources: manifest.ResourcesSpec{}}}
	provisioner := &recordProvisioner{}
	waiter := &recordWaiter{}

	workflow := NewUpWorkflow(
		builder,
		upper,
		downer,
		publisher,
		creds,
		loader,
		parser,
		provisioner,
		waiter,
		envApplier,
		ui,
	)

	req := UpRequest{
		Context:      ctx,
		Env:          "dev",
		TemplatePath: "/repo/template.yaml",
		Detach:       true,
		Wait:         true,
		Reset:        true,
		EnvFile:      "/repo/.env",
	}
	result, err := workflow.Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(envApplier.calls) != 1 {
		t.Fatalf("expected env applier to be called once, got %d", len(envApplier.calls))
	}
	if len(downer.calls) != 1 || downer.calls[0].project != ctx.ComposeProject || !downer.calls[0].volumes {
		t.Fatalf("expected downer reset call")
	}
	if len(builder.requests) != 1 {
		t.Fatalf("expected builder to be called")
	}
	if len(upper.requests) != 1 {
		t.Fatalf("expected upper to be called")
	}
	upReq := upper.requests[0]
	if upReq.Detach != req.Detach || upReq.Wait != req.Wait || upReq.EnvFile != req.EnvFile {
		t.Fatalf("upper request mismatch")
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
	if len(waiter.calls) != 1 {
		t.Fatalf("expected waiter to be called")
	}
	if !reflect.DeepEqual(result.Ports, publisher.ports) {
		t.Fatalf("expected ports to be returned")
	}
	if result.Credentials.AuthUser != "user" || !result.Credentials.Generated {
		t.Fatalf("expected credentials to be returned")
	}
	if len(ui.blocks) != 2 {
		t.Fatalf("expected credentials and ports blocks")
	}
	if len(ui.successes) != 1 || !strings.Contains(ui.successes[0], "Up complete") {
		t.Fatalf("expected success message")
	}
}

func TestUpWorkflowPortPublisherErrorWarns(t *testing.T) {
	ctx := state.Context{ComposeProject: "esb-dev"}
	ui := &testUI{}
	publisher := &recordPortPublisher{err: errors.New("port failure")}
	upper := &recordUpper{}
	loader := &recordTemplateLoader{content: "template"}
	parser := &recordTemplateParser{result: generator.ParseResult{Resources: manifest.ResourcesSpec{}}}
	provisioner := &recordProvisioner{}

	workflow := NewUpWorkflow(
		nil,
		upper,
		nil,
		publisher,
		nil,
		loader,
		parser,
		provisioner,
		nil,
		nil,
		ui,
	)
	_, err := workflow.Run(UpRequest{Context: ctx, TemplatePath: "/repo/template.yaml"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ui.warns) != 1 || !strings.Contains(ui.warns[0], "port failure") {
		t.Fatalf("expected warning for port publish error")
	}
}

func TestUpWorkflowMissingDownerOnReset(t *testing.T) {
	workflow := NewUpWorkflow(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := workflow.Run(UpRequest{Context: state.Context{}, Reset: true})
	if err == nil || !strings.Contains(err.Error(), "downer not configured") {
		t.Fatalf("expected downer missing error, got %v", err)
	}
}

func TestUpWorkflowMissingBuilderOnBuild(t *testing.T) {
	workflow := NewUpWorkflow(nil, &recordUpper{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := workflow.Run(UpRequest{Context: state.Context{}, Build: true})
	if err == nil || !strings.Contains(err.Error(), "builder not configured") {
		t.Fatalf("expected builder missing error, got %v", err)
	}
}

func TestUpWorkflowMissingTemplatePipeline(t *testing.T) {
	workflow := NewUpWorkflow(nil, &recordUpper{}, nil, nil, nil, nil, nil, &recordProvisioner{}, nil, nil, nil)
	_, err := workflow.Run(UpRequest{Context: state.Context{}})
	if err == nil || !strings.Contains(err.Error(), "template pipeline not configured") {
		t.Fatalf("expected template pipeline error, got %v", err)
	}
}
