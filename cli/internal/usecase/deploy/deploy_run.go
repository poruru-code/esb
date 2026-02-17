// Where: cli/internal/usecase/deploy/deploy_run.go
// What: Workflow.Run orchestration skeleton.
// Why: Keep deploy phase order visible while details live in dedicated files.
package deploy

import (
	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

type generateResult struct {
	stagingDir  string
	preSnapshot domaincfg.Snapshot
}

// Run executes the deploy workflow.
func (w Workflow) Run(req Request) error {
	if w.Build == nil {
		return errBuilderNotConfigured
	}
	if w.ComposeRunner == nil {
		return errComposeRunnerNotConfigured
	}

	imagePrewarm, err := NormalizeImagePrewarmMode(req.ImagePrewarm)
	if err != nil {
		return err
	}

	req = w.alignGatewayRuntime(req)
	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	generated, err := w.runGeneratePhase(req)
	if err != nil {
		return err
	}
	if !req.BuildOnly {
		if err := w.runApplyPhase(req, generated.stagingDir, imagePrewarm); err != nil {
			return err
		}
	}

	// For containerd mode, function images are pulled by agent/runtime-node.
	// This ensures proper image store management in containerd environments.
	// See: agent/runtime-node IMAGE_PULL_POLICY configuration.

	if w.UserInterface != nil {
		w.UserInterface.Success(w.successMessage(req))
	}
	return nil
}

// Apply executes apply-only deploy flow (no build/generation).
func (w Workflow) Apply(req Request) error {
	if w.ComposeRunner == nil {
		return errComposeRunnerNotConfigured
	}

	imagePrewarm, err := NormalizeImagePrewarmMode(req.ImagePrewarm)
	if err != nil {
		return err
	}

	req = w.alignGatewayRuntime(req)
	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	stagingDir, err := staging.ConfigDir(req.TemplatePath, req.Context.ComposeProject, req.Env)
	if err != nil {
		return err
	}
	if err := w.runApplyPhase(req, stagingDir, imagePrewarm); err != nil {
		return err
	}

	if w.UserInterface != nil {
		w.UserInterface.Success(w.successMessage(req))
	}
	return nil
}

// runGeneratePhase executes build/generation-side deploy steps.
func (w Workflow) runGeneratePhase(req Request) (generateResult, error) {
	generated, err := w.prepareGenerate(req)
	if err != nil {
		return generateResult{}, err
	}
	if err := w.runBuildPhase(req); err != nil {
		return generateResult{}, err
	}
	w.emitPostBuildSummary(req, generated.stagingDir, generated.preSnapshot)
	return generated, nil
}

// runApplyPhase executes runtime/provision-side deploy steps.
func (w Workflow) runApplyPhase(req Request, stagingDir, imagePrewarm string) error {
	if err := w.waitRegistryAndServices(req); err != nil {
		return err
	}
	return w.runRuntimeProvisionPhase(req, stagingDir, imagePrewarm)
}

// prepareGenerate resolves and snapshots generate outputs before build.
func (w Workflow) prepareGenerate(req Request) (generateResult, error) {
	stagingDir, preSnapshot, err := w.prepareBuildPhase(req)
	if err != nil {
		return generateResult{}, err
	}
	return generateResult{
		stagingDir:  stagingDir,
		preSnapshot: preSnapshot,
	}, nil
}
