// Where: cli/internal/usecase/deploy/deploy_runtime_provision.go
// What: Runtime sync, image prewarm, and provisioner phase helpers.
// Why: Keep post-build runtime operations isolated from workflow skeleton.
package deploy

import (
	"context"
	"fmt"
	"path/filepath"
)

func (w Workflow) runRuntimeProvisionPhase(req Request, stagingDir, imagePrewarm string) error {
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
	if err := w.syncRuntimeConfig(req); err != nil {
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
