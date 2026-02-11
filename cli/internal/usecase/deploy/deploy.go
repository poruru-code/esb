// Where: cli/internal/usecase/deploy/deploy.go
// What: Deploy workflow types and constructor.
// Why: Keep public workflow contract stable while splitting phase implementations.
package deploy

import (
	"errors"
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/build"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
)

var (
	errBuilderNotConfigured       = errors.New("builder is not configured")
	errComposeRunnerNotConfigured = errors.New("compose runner is not configured")
	errDockerClientNotConfigured  = errors.New("docker client is not configured")
	errRegistryNotResponding      = errors.New("registry not responding")
)

// Request captures the inputs required to run a deploy.
type Request struct {
	Context        state.Context
	Env            string
	TemplatePath   string
	Mode           string
	OutputDir      string
	Parameters     map[string]string
	Tag            string
	NoCache        bool
	NoDeps         bool
	Verbose        bool
	ComposeFiles   []string
	BuildOnly      bool
	BundleManifest bool
	ImagePrewarm   string
	Emoji          bool
}

// Workflow executes the deploy orchestration steps.
type Workflow struct {
	Build              func(build.BuildRequest) error
	ApplyRuntimeEnv    func(state.Context) error
	UserInterface      ui.UserInterface
	ComposeRunner      compose.CommandRunner
	ComposeProvisioner ComposeProvisioner
	RegistryWaiter     RegistryWaiter
	DockerClient       DockerClientFactory
}

// ComposeProvisioner defines compose-related operational behavior consumed by the workflow.
type ComposeProvisioner interface {
	CheckServicesStatus(composeProject, mode string)
	RunProvisioner(
		composeProject string,
		mode string,
		noDeps bool,
		verbose bool,
		projectDir string,
		composeFiles []string,
	) error
}

// DockerClientFactory constructs Docker SDK clients used by runtime inspection paths.
type DockerClientFactory func() (compose.DockerClient, error)

// NewDeployWorkflow constructs a Workflow.
func NewDeployWorkflow(
	build func(build.BuildRequest) error,
	applyRuntimeEnv func(state.Context) error,
	ui ui.UserInterface,
	composeRunner compose.CommandRunner,
) Workflow {
	return Workflow{
		Build:           build,
		ApplyRuntimeEnv: applyRuntimeEnv,
		UserInterface:   ui,
		ComposeRunner:   composeRunner,
		RegistryWaiter:  defaultRegistryWaiter,
	}
}

func (w Workflow) buildRequest(req Request) build.BuildRequest {
	return build.BuildRequest{
		ProjectDir:   req.Context.ProjectDir,
		ProjectName:  req.Context.ComposeProject,
		TemplatePath: req.TemplatePath,
		Env:          req.Env,
		Mode:         req.Mode,
		OutputDir:    req.OutputDir,
		Parameters:   req.Parameters,
		Tag:          req.Tag,
		NoCache:      req.NoCache,
		Verbose:      req.Verbose,
		Bundle:       req.BundleManifest,
		Emoji:        req.Emoji,
	}
}

func (w Workflow) successMessage(req Request) string {
	if req.BuildOnly {
		return "Build complete"
	}
	return "Deploy complete"
}

func (w Workflow) wrapProvisionerError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("provisioner failed: %w", err)
}
