// Where: cli/internal/usecase/deploy/deploy_runtime_provision.go
// What: Runtime sync, image prewarm, and provisioner phase helpers.
// Why: Keep post-build runtime operations isolated from workflow skeleton.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var errArtifactPathRequired = errors.New("artifact path is required for apply phase")

func (w Workflow) runRuntimeProvisionPhase(req Request, stagingDir, imagePrewarm string) error {
	if err := w.applyArtifactRuntimeConfig(req, stagingDir); err != nil {
		return err
	}
	manifestPath := filepath.Join(stagingDir, "image-import.json")
	manifest, exists, err := loadImageImportManifest(manifestPath)
	if err != nil {
		return err
	}
	if exists && len(manifest.Images) > 0 {
		if imagePrewarm != "all" {
			return fmt.Errorf("image prewarm is required for templates with image functions (use --image-prewarm=all)")
		}
	}
	if imagePrewarm == "all" {
		if err := runImagePrewarm(
			context.Background(),
			w.ComposeRunner,
			w.UserInterface,
			manifestPath,
			req.Verbose,
		); err != nil {
			return err
		}
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
