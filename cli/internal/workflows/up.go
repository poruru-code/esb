// Where: cli/internal/workflows/up.go
// What: Up workflow orchestration.
// Why: Keep CLI commands thin while hosting the business logic in workflows.
package workflows

import (
	"context"
	"errors"
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/presenters"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// UpRequest captures the inputs required for the Up workflow.
type UpRequest struct {
	Context      state.Context
	Env          string
	TemplatePath string
	Detach       bool
	Wait         bool
	Build        bool
	Reset        bool
	EnvFile      string
}

// UpResult contains feedback returned by the workflow.
type UpResult struct {
	Ports       ports.PortPublishResult
	Credentials ports.AuthCredentials
}

// UpWorkflow orchestrates dependencies required to bring the environment up.
type UpWorkflow struct {
	Builder           ports.Builder
	Upper             ports.Upper
	Downer            ports.Downer
	PortPublisher     ports.PortPublisher
	CredentialManager ports.CredentialManager
	TemplateLoader    ports.TemplateLoader
	TemplateParser    ports.TemplateParser
	Provisioner       ports.Provisioner
	Waiter            ports.GatewayWaiter
	EnvApplier        ports.RuntimeEnvApplier
	UserInterface     ports.UserInterface
}

// NewUpWorkflow constructs an UpWorkflow.
func NewUpWorkflow(builder ports.Builder, upper ports.Upper, downer ports.Downer, publisher ports.PortPublisher,
	credentialMgr ports.CredentialManager, loader ports.TemplateLoader, parser ports.TemplateParser,
	provisioner ports.Provisioner, waiter ports.GatewayWaiter, envApplier ports.RuntimeEnvApplier, ui ports.UserInterface,
) UpWorkflow {
	return UpWorkflow{
		Builder:           builder,
		Upper:             upper,
		Downer:            downer,
		PortPublisher:     publisher,
		CredentialManager: credentialMgr,
		TemplateLoader:    loader,
		TemplateParser:    parser,
		Provisioner:       provisioner,
		Waiter:            waiter,
		EnvApplier:        envApplier,
		UserInterface:     ui,
	}
}

// Run executes the Up workflow with the given request.
func (w UpWorkflow) Run(req UpRequest) (UpResult, error) {
	var result UpResult

	if w.EnvApplier != nil {
		w.EnvApplier.Apply(req.Context)
	}

	if req.Reset {
		presenters.PrintResetWarning(w.UserInterface)
		if w.Downer == nil {
			return result, errors.New("downer not configured")
		}
		if err := w.Downer.Down(req.Context.ComposeProject, true); err != nil {
			return result, err
		}
	}

	if w.CredentialManager != nil {
		result.Credentials = w.CredentialManager.Ensure()
		if result.Credentials.Generated {
			presenters.PrintCredentials(w.UserInterface, result.Credentials)
		}
	}

	if req.Build || req.Reset {
		if w.Builder == nil {
			return result, errors.New("builder not configured")
		}
		if err := w.Builder.Build(generator.BuildRequest{
			ProjectDir:   req.Context.ProjectDir,
			ProjectName:  req.Context.ComposeProject,
			TemplatePath: req.TemplatePath,
			Env:          req.Env,
		}); err != nil {
			return result, err
		}
	}

	if w.Upper == nil {
		return result, errors.New("upper not configured")
	}
	if err := w.Upper.Up(ports.UpRequest{
		Context: req.Context,
		Detach:  req.Detach,
		Wait:    req.Wait,
		EnvFile: req.EnvFile,
	}); err != nil {
		return result, err
	}

	if w.PortPublisher != nil {
		publishResult, err := w.PortPublisher.Publish(req.Context)
		if err != nil {
			if w.UserInterface != nil {
				w.UserInterface.Warn(err.Error())
			}
		} else {
			result.Ports = publishResult
		}
	}

	if w.TemplateLoader == nil || w.TemplateParser == nil || w.Provisioner == nil {
		return result, errors.New("template pipeline not configured")
	}
	content, err := w.TemplateLoader.Read(req.TemplatePath)
	if err != nil {
		return result, fmt.Errorf("failed to read template: %w", err)
	}
	parsed, err := w.TemplateParser.Parse(content, nil)
	if err != nil {
		return result, fmt.Errorf("failed to parse template: %w", err)
	}
	if err := w.Provisioner.Apply(context.Background(), parsed.Resources, req.Context.ComposeProject); err != nil {
		return result, err
	}

	if req.Wait && w.Waiter != nil {
		if err := w.Waiter.Wait(req.Context); err != nil {
			return result, err
		}
	}

	if w.UserInterface != nil {
		w.UserInterface.Success("✓ Up complete")
		w.UserInterface.Info("Next:")
		w.UserInterface.Info("  esb logs <service>  # View logs")
		w.UserInterface.Info("  esb down            # Stop environment")
		presenters.PrintDiscoveredPorts(w.UserInterface, result.Ports.Published)
	}

	return result, nil
}
