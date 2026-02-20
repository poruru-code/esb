// Where: cli/internal/usecase/deploy/deploy_runtime_provision.go
// What: Runtime sync and provisioner phase helpers.
// Why: Keep post-build runtime operations isolated from workflow skeleton.
package deploy

import (
	"fmt"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

func (w Workflow) runRuntimeProvisionPhase(req Request, stagingDir string) error {
	if err := w.applyArtifactRuntimeConfig(req, stagingDir); err != nil {
		return err
	}
	if err := w.syncRuntimeConfigFromDir(req.Context.ComposeProject, stagingDir); err != nil {
		return err
	}
	return w.wrapProvisionerError(w.runProvisioner(
		req.Context.ComposeProject,
		req.Context.Mode,
		req.NoDeps,
		req.Verbose,
		req.Context.ProjectDir,
		req.ComposeFiles,
	))
}

func (w Workflow) applyArtifactRuntimeConfig(req Request, stagingDir string) error {
	observation, observationWarnings := w.resolveRuntimeObservation(req)
	result, err := artifactcore.ExecuteApply(artifactcore.ApplyInput{
		ArtifactPath:  req.ArtifactPath,
		OutputDir:     stagingDir,
		SecretEnvPath: req.SecretEnvPath,
		Runtime:       observation,
	})
	if err != nil {
		return fmt.Errorf("apply artifact runtime config: %w", err)
	}
	if w.UserInterface != nil {
		for _, warning := range observationWarnings {
			w.UserInterface.Warn(fmt.Sprintf("Warning: %s", warning))
		}
		for _, warning := range result.Warnings {
			w.UserInterface.Warn(fmt.Sprintf("Warning: %s", warning))
		}
	}
	return nil
}
