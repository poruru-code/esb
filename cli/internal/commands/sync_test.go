// Where: cli/internal/commands/sync_test.go
// What: Tests for sync command wiring.
// Why: Ensure sync command invokes the sync workflow with resolved context.
package commands

import (
	"bytes"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type fakePortPublisher struct {
	calls []state.Context
}

func (f *fakePortPublisher) Publish(ctx state.Context) (ports.PortPublishResult, error) {
	f.calls = append(f.calls, ctx)
	return ports.PortPublishResult{}, nil
}

func TestRunSyncCallsWorkflow(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeGeneratorFixture(projectDir, "default"); err != nil {
		t.Fatalf("write generator fixture: %v", err)
	}
	setupProjectConfig(t, projectDir, "demo")

	publisher := &fakePortPublisher{}
	loader := &fakeUpTemplateLoader{content: "test"}
	parser := &fakeUpTemplateParser{result: generator.ParseResult{Resources: manifest.ResourcesSpec{}}}
	provisioner := &fakeProvisioner{}

	var out bytes.Buffer
	deps := Dependencies{
		Out:        &out,
		ProjectDir: projectDir,
		Sync: SyncDeps{
			PortPublisher:  publisher,
			TemplateLoader: loader,
			TemplateParser: parser,
			Provisioner:    provisioner,
		},
	}

	exitCode := Run([]string{"sync"}, deps)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if len(publisher.calls) != 1 {
		t.Fatalf("expected port publisher to be called")
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner to be called")
	}
}

// Re-defining fakes if not available from up_test.go
// Actually they should be available if in the same package during test.
// But up_test.go uses 'record' prefixes in workflows_test.go? No, up_test.go has its own.
// Let's define minimal local ones to be sure of isolation.

type fakeUpTemplateLoader struct {
	content string
}

func (f *fakeUpTemplateLoader) Read(string) (string, error) {
	return f.content, nil
}

type fakeUpTemplateParser struct {
	result generator.ParseResult
}

func (f *fakeUpTemplateParser) Parse(string, map[string]string) (generator.ParseResult, error) {
	return f.result, nil
}
