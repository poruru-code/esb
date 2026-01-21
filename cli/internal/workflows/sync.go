// Where: cli/internal/workflows/sync.go
// What: Sync workflow orchestration.
// Why: Keep CLI commands thin while hosting the business logic in workflows.
package workflows

import (
	"context"
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/presenters"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// SyncRequest captures the inputs required for the Sync workflow.
type SyncRequest struct {
	Context      state.Context
	Env          string
	TemplatePath string
	Wait         bool
}

// SyncResult contains feedback returned by the workflow.
type SyncResult struct {
	Ports ports.PortPublishResult
}

// SyncWorkflow orchestrates dependencies required to sync the environment.
type SyncWorkflow struct {
	PortDiscoverer ports.StateStore
	// Note: We use helpers.PortDiscoverer directly in the command usually via helpers.DiscoverAndPersistPorts
	// taking in a Discoverer. But sticking to the pattern, we likely want injected dependencies.
	// Looking at UpWorkflow, it embeds ports.PortPublisher.
	PortPublisher  ports.PortPublisher
	TemplateLoader ports.TemplateLoader
	TemplateParser ports.TemplateParser
	Provisioner    ports.Provisioner
	UserInterface  ports.UserInterface
}

// NewSyncWorkflow constructs a SyncWorkflow.
func NewSyncWorkflow(
	publisher ports.PortPublisher,
	loader ports.TemplateLoader,
	parser ports.TemplateParser,
	provisioner ports.Provisioner,
	ui ports.UserInterface,
) SyncWorkflow {
	return SyncWorkflow{
		PortPublisher:  publisher,
		TemplateLoader: loader,
		TemplateParser: parser,
		Provisioner:    provisioner,
		UserInterface:  ui,
	}
}

// Run executes the Sync workflow with the given request.
func (w SyncWorkflow) Run(req SyncRequest) (SyncResult, error) {
	var result SyncResult

	// 1. Port Discovery & Persistence
	if w.PortPublisher != nil {
		publishResult, err := w.PortPublisher.Publish(req.Context)
		if err != nil {
			if w.UserInterface != nil {
				w.UserInterface.Warn(fmt.Sprintf("Port discovery warning: %v", err))
			}
		} else {
			result.Ports = publishResult
			if w.UserInterface != nil && publishResult.Changed {
				w.UserInterface.Info("Ports updated.")
			}
		}
	}

	// 2. Provisioning
	// Only proceed if provisioner is configured (it might be optional if just syncing ports?)
	// But CLI design said "Sync resources".
	if w.Provisioner != nil {
		if w.TemplateLoader == nil || w.TemplateParser == nil {
			return result, fmt.Errorf("template pipeline not configured")
		}

		if w.UserInterface != nil {
			w.UserInterface.Info(fmt.Sprintf("Reading template: %s", req.TemplatePath))
		}

		content, err := w.TemplateLoader.Read(req.TemplatePath)
		if err != nil {
			return result, fmt.Errorf("failed to read template: %w", err)
		}

		parsed, err := w.TemplateParser.Parse(content, nil)
		if err != nil {
			return result, fmt.Errorf("failed to parse template: %w", err)
		}

		if w.UserInterface != nil {
			w.UserInterface.Info("Provisioning resources...")
		}

		err = w.Provisioner.Apply(context.Background(), parsed.Resources, req.Context.ComposeProject)
		if err != nil {
			return result, fmt.Errorf("provisioning failed: %w", err)
		}

		if w.UserInterface != nil {
			w.UserInterface.Success("Resources provisioned.")
		}
	} else if w.UserInterface != nil {
		w.UserInterface.Warn("Provisioner not configured. Skipping resource sync.")
	}

	if w.UserInterface != nil {
		w.UserInterface.Success("✓ Sync complete")
		presenters.PrintDiscoveredPorts(w.UserInterface, result.Ports.Published)
	}

	return result, nil
}

// Helper to reuse print logic?
// Ideally `PrintDiscoveredPorts` should be shared.
// Done: moved to `presenters` package.
