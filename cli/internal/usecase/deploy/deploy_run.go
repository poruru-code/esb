// Where: cli/internal/usecase/deploy/deploy_run.go
// What: Workflow.Run orchestration skeleton.
// Why: Keep deploy phase order visible while details live in dedicated files.
package deploy

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

	if err := w.waitRegistryAndServices(req); err != nil {
		return err
	}

	stagingDir, preSnapshot, err := w.prepareBuildPhase(req)
	if err != nil {
		return err
	}
	if err := w.runBuildPhase(req); err != nil {
		return err
	}
	w.emitPostBuildSummary(req, stagingDir, preSnapshot)

	if !req.BuildOnly {
		if err := w.runRuntimeProvisionPhase(req, stagingDir, imagePrewarm); err != nil {
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
