// Where: cli/internal/workflows/build.go
// What: Build workflow orchestration.
// Why: Encapsulate build-specific logic without CLI concerns.
package workflows

import (
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// BuildRequest captures the inputs required to run a build.
type BuildRequest struct {
	Context      state.Context
	Env          string
	TemplatePath string
	NoCache      bool
	Verbose      bool
}

// BuildWorkflow executes the build orchestration steps.
type BuildWorkflow struct {
	Builder       ports.Builder
	EnvApplier    ports.RuntimeEnvApplier
	UserInterface ports.UserInterface
}

// NewBuildWorkflow constructs a BuildWorkflow.
func NewBuildWorkflow(builder ports.Builder, envApplier ports.RuntimeEnvApplier, ui ports.UserInterface) BuildWorkflow {
	return BuildWorkflow{
		Builder:       builder,
		EnvApplier:    envApplier,
		UserInterface: ui,
	}
}

// Run executes the build workflow.
func (w BuildWorkflow) Run(req BuildRequest) error {
	if w.Builder == nil {
		return fmt.Errorf("builder port is not configured")
	}
	if w.EnvApplier != nil {
		w.EnvApplier.Apply(req.Context)
	}

	buildRequest := generator.BuildRequest{
		ProjectDir:   req.Context.ProjectDir,
		ProjectName:  req.Context.ComposeProject,
		TemplatePath: req.TemplatePath,
		Env:          req.Env,
		NoCache:      req.NoCache,
		Verbose:      req.Verbose,
	}

	if err := w.Builder.Build(buildRequest); err != nil {
		return err
	}

	if w.UserInterface != nil {
		w.UserInterface.Success("✓ Build complete")
	}
	return nil
}
