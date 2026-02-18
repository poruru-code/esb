// Where: cli/internal/usecase/deploy/deploy_run.go
// What: Workflow.Run orchestration skeleton.
// Why: Keep deploy phase order visible while details live in dedicated files.
package deploy

import (
	"os"
)

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

	if err := w.runGeneratePhase(req); err != nil {
		return err
	}
	if !req.BuildOnly {
		if err := w.runApplyPhase(req, imagePrewarm); err != nil {
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

	if err := w.runApplyPhase(req, imagePrewarm); err != nil {
		return err
	}

	if w.UserInterface != nil {
		w.UserInterface.Success(w.successMessage(req))
	}
	return nil
}

// runGeneratePhase executes build/generation-side deploy steps.
func (w Workflow) runGeneratePhase(req Request) error {
	if err := w.runBuildPhase(req); err != nil {
		return err
	}
	w.emitPostBuildSummary(req)
	return nil
}

// runApplyPhase executes runtime/provision-side deploy steps.
func (w Workflow) runApplyPhase(req Request, imagePrewarm string) error {
	stagingDir, cleanup, err := createApplyWorkspace()
	if err != nil {
		return err
	}
	defer cleanup()
	if err := w.waitRegistryAndServices(req); err != nil {
		return err
	}
	return w.runRuntimeProvisionPhase(req, stagingDir, imagePrewarm)
}

func createApplyWorkspace() (string, func(), error) {
	dir, err := os.MkdirTemp("", "esb-runtime-config-*")
	if err != nil {
		return "", nil, err
	}
	return dir, func() {
		_ = os.RemoveAll(dir)
	}, nil
}
