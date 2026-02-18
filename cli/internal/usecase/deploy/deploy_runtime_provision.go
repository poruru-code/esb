// Where: cli/internal/usecase/deploy/deploy_runtime_provision.go
// What: Runtime sync and provisioner phase helpers.
// Why: Keep post-build runtime operations isolated from workflow skeleton.
package deploy

import (
	"errors"
	"strings"
)

var errArtifactPathRequired = errors.New("artifact path is required for apply phase")

func (w Workflow) runRuntimeProvisionPhase(req Request, stagingDir string) error {
	if err := w.applyArtifactRuntimeConfig(req, stagingDir); err != nil {
		return err
	}
	if err := w.syncRuntimeConfigFromDir(req.Context.ComposeProject, stagingDir); err != nil {
		return err
	}
	return w.wrapProvisionerError(w.runProvisioner(
		req.Context.ComposeProject,
		req.Mode,
		req.NoDeps,
		req.Verbose,
		req.Context.ProjectDir,
		req.ComposeFiles,
	))
}

func (w Workflow) applyArtifactRuntimeConfig(req Request, stagingDir string) error {
	artifactPath := strings.TrimSpace(req.ArtifactPath)
	if artifactPath == "" {
		return errArtifactPathRequired
	}
	return ApplyArtifact(ArtifactApplyRequest{
		ArtifactPath: artifactPath,
		OutputDir:    stagingDir,
	})
}
